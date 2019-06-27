###
Represents a single:

   client <-- webrtc --> snowflake <-- websocket --> relay

Every ProxyPair has a Snowflake ID, which is necessary when responding to the
Broker with an WebRTC answer.
###

class ProxyPair
  MAX_BUFFER: 10 * 1024 * 1024
  pc:          null
  client:      null  # WebRTC Data channel
  relay:       null   # websocket
  timer:       0
  running:     true
  active:      false  # Whether serving a client.
  flush_timeout_id: null
  onCleanup:   null
  id:          null

  ###
  Constructs a ProxyPair where:
  - @relayAddr is the destination relay
  - @rateLimit specifies a rate limit on traffic
  ###
  constructor: (@relayAddr, @rateLimit, @pcConfig) ->
    @id = Util.genSnowflakeID()
    @c2rSchedule = []
    @r2cSchedule = []

  # Prepare a WebRTC PeerConnection and await for an SDP offer.
  begin: ->
    @pc = new PeerConnection @pcConfig, {
      optional: [
        { DtlsSrtpKeyAgreement: true }
        { RtpDataChannels: false }
      ] }
    @pc.onicecandidate = (evt) =>
      # Browser sends a null candidate once the ICE gathering completes.
      if null == evt.candidate
        # TODO: Use a promise.all to tell Snowflake about all offers at once,
        # once multiple proxypairs are supported.
        dbg 'Finished gathering ICE candidates.'
        snowflake.broker.sendAnswer @id, @pc.localDescription
    # OnDataChannel triggered remotely from the client when connection succeeds.
    @pc.ondatachannel = (dc) =>
      channel = dc.channel
      dbg 'Data Channel established...'
      @prepareDataChannel channel
      @client = channel

  receiveWebRTCOffer: (offer) ->
    if 'offer' != offer.type
      log 'Invalid SDP received -- was not an offer.'
      return false
    try
      err = @pc.setRemoteDescription offer
    catch e
      log 'Invalid SDP message.'
      return false
    dbg 'SDP ' + offer.type + ' successfully received.'
    true

  # Given a WebRTC DataChannel, prepare callbacks.
  prepareDataChannel: (channel) =>
    channel.onopen = =>
      log 'WebRTC DataChannel opened!'
      snowflake.state = Snowflake.MODE.WEBRTC_READY
      snowflake.ui.setActive true
      # This is the point when the WebRTC datachannel is done, so the next step
      # is to establish websocket to the server.
      @connectRelay()
    channel.onclose = =>
      log 'WebRTC DataChannel closed.'
      snowflake.ui.setStatus 'disconnected by webrtc.'
      snowflake.ui.setActive false
      snowflake.state = Snowflake.MODE.INIT
      @flush()
      @close()
      # TODO: Change this for multiplexing.
      snowflake.reset()
    channel.onerror = -> log 'Data channel error!'
    channel.binaryType = "arraybuffer"
    channel.onmessage = @onClientToRelayMessage

  # Assumes WebRTC datachannel is connected.
  connectRelay: =>
    dbg 'Connecting to relay...'

    # Get a remote IP address from the PeerConnection, if possible. Add it to
    # the WebSocket URL's query string if available.
    # MDN marks remoteDescription as "experimental". However the other two
    # options, currentRemoteDescription and pendingRemoteDescription, which
    # are not marked experimental, were undefined when I tried them in Firefox
    # 52.2.0.
    # https://developer.mozilla.org/en-US/docs/Web/API/RTCPeerConnection/remoteDescription
    peer_ip = Parse.ipFromSDP(@pc.remoteDescription?.sdp)
    params = []
    if peer_ip?
      params.push(["client_ip", peer_ip])

    @relay = WS.makeWebsocket @relayAddr, params
    @relay.label = 'websocket-relay'
    @relay.onopen = =>
      if @timer
        clearTimeout @timer
        @timer = 0
      log @relay.label + ' connected!'
      snowflake.ui.setStatus 'connected'
    @relay.onclose = =>
      log @relay.label + ' closed.'
      snowflake.ui.setStatus 'disconnected.'
      snowflake.ui.setActive false
      snowflake.state = Snowflake.MODE.INIT
      @flush()
      @close()
    @relay.onerror = @onError
    @relay.onmessage = @onRelayToClientMessage
    # TODO: Better websocket timeout handling.
    @timer = setTimeout((=>
      return if 0 == @timer
      log @relay.label + ' timed out connecting.'
      @relay.onclose()), 5000)

  # WebRTC --> websocket
  onClientToRelayMessage: (msg) =>
    dbg 'WebRTC --> websocket data: ' + msg.data.byteLength + ' bytes'
    @c2rSchedule.push msg.data
    @flush()

  # websocket --> WebRTC
  onRelayToClientMessage: (event) =>
    dbg 'websocket --> WebRTC data: ' + event.data.byteLength + ' bytes'
    @r2cSchedule.push event.data
    @flush()

  onError: (event) =>
    ws = event.target
    log ws.label + ' error.'
    @close()

  # Close both WebRTC and websocket.
  close: ->
    if @timer
      clearTimeout @timer
      @timer = 0
    @running = false
    @client.close() if @webrtcIsReady()
    @relay.close() if @relayIsReady()
    relay = null

  # Send as much data in both directions as the rate limit currently allows.
  flush: =>
    clearTimeout @flush_timeout_id if @flush_timeout_id
    @flush_timeout_id = null
    busy = true
    checkChunks = =>
      busy = false
      # WebRTC --> websocket
      if @relayIsReady() &&
         @relay.bufferedAmount < @MAX_BUFFER &&
         @c2rSchedule.length > 0
        chunk = @c2rSchedule.shift()
        @rateLimit.update chunk.byteLength
        @relay.send chunk
        busy = true
      # websocket --> WebRTC
      if @webrtcIsReady() &&
         @client.bufferedAmount < @MAX_BUFFER &&
         @r2cSchedule.length > 0
        chunk = @r2cSchedule.shift()
        @rateLimit.update chunk.byteLength
        @client.send chunk
        busy = true

    checkChunks() while busy  && !@rateLimit.isLimited()

    if @r2cSchedule.length > 0 || @c2rSchedule.length > 0 ||
       (@relayIsReady()  && @relay.bufferedAmount > 0) ||
       (@webrtcIsReady() && @client.bufferedAmount > 0)
      @flush_timeout_id = setTimeout @flush,  @rateLimit.when() * 1000

  webrtcIsReady: -> null != @client && 'open' == @client.readyState
  relayIsReady: -> (null != @relay) && (WebSocket.OPEN == @relay.readyState)
  isClosed: (ws) -> undefined == ws || WebSocket.CLOSED == ws.readyState
