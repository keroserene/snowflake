###
Represents a single:

   client <-- webrtc --> snowflake <-- websocket --> relay

###


class ProxyPair

  MAX_BUFFER: 10 * 1024 * 1024
  pc: null
  c2rSchedule: []
  r2cSchedule: []
  client: null  # WebRTC Data channel
  relay: null   # websocket
  running: true
  flush_timeout_id: null

  constructor: (@clientAddr, @relayAddr, @rateLimit) ->

  # Prepare a WebRTC PeerConnection and await for an SDP offer.
  begin: ->
    @pc = new PeerConnection config, {
      optional: [
        { DtlsSrtpKeyAgreement: true }
        { RtpDataChannels: false }
      ] }
    @pc.onicecandidate = (evt) =>
      # Browser sends a null candidate once the ICE gathering completes.
      # In this case, it makes sense to send one copy-paste blob.
      if null == evt.candidate
        # TODO: Use a promise.all to tell Snowflake about all offers at once,
        # once multiple proxypairs are supported.
        log 'Finished gathering ICE candidates.'
        if COPY_PASTE_ENABLED
          Signalling.send @pc.localDescription
        else
          snowflake.broker.sendAnswer @pc.localDescription
    # OnDataChannel triggered remotely from the client when connection succeeds.
    @pc.ondatachannel = (dc) =>
      console.log dc
      channel = dc.channel
      log 'Data Channel established...'
      @prepareDataChannel channel
      @client = channel

  receiveWebRTCOffer: (offer) ->
    console.assert 'offer' == offer.type
    try
      err = @pc.setRemoteDescription offer
    catch e
      log 'Invalid SDP message.'
      return false
    log 'SDP ' + offer.type + ' successfully received.'
    true

  prepareDataChannel: (channel) =>
    channel.onopen = =>
      log 'Data channel opened!'
      snowflake.state = MODE.WEBRTC_READY
      $msglog.className = 'active' if $msglog
      # This is the point when the WebRTC datachannel is done, so the next step
      # is to establish websocket to the server.
      @connectRelay()
    channel.onclose = ->
      log 'Data channel closed.'
      Status.set 'disconnected.'
      snowflake.state = MODE.INIT
      $msglog.className = '' if $msglog
      # Change this for multiplexing.
      snowflake.reset()
    channel.onerror = -> log 'Data channel error!'
    channel.onmessage = @onClientToRelayMessage

  # Assumes WebRTC datachannel is connected.
  connectRelay: =>
    log 'Connecting to relay...'
    @relay = makeWebsocket @relayAddr
    @relay.label = 'websocket-relay'
    @relay.onopen = =>
      log '\nRelay ' + @relay.label + ' connected!'
      Status.set 'connected'
    @relay.onclose = @onClose
    @relay.onerror = @onError
    @relay.onmessage = @onRelayToClientMessage

  # WebRTC --> websocket
  onClientToRelayMessage: (msg) =>
    line = recv = msg.data
    if DEBUG
      # Go sends only raw bytes...
      if '[object ArrayBuffer]' == recv.toString()
        bytes = new Uint8Array recv
        line = String.fromCharCode.apply(null, bytes)
      line = line.trim()
      console.log 'WebRTC --> websocket data: ' + line
    @c2rSchedule.push recv
    @flush()

  # websocket --> WebRTC
  onRelayToClientMessage: (event) =>
    @r2cSchedule.push event.data
    # log 'websocket-->WebRTC data: ' + event.data
    @flush()

  onClose: (event) =>
    ws = event.target
    log(ws.label + ': closed.')
    @flush()
    @maybeCleanup()

  onError: (event) =>
    ws = event.target
    log ws.label + ': error.'
    @close()
    # we can't rely on onclose_callback to cleanup, since one common error
    # case is when the client fails to connect and the relay never starts.
    # in that case close() is a NOP and onclose_callback is never called.
    @maybeCleanup()

  webrtcIsReady: -> null != @client && 'open' == @client.readyState
  relayIsReady: -> (null != @relay) && (WebSocket.OPEN == @relay.readyState)
  isClosed: (ws) -> undefined == ws || WebSocket.CLOSED == ws.readyState
  close: ->
    @client.close() if @webrtcIsReady()
    @relay.close() if @relayIsReady()
    relay = null

  maybeCleanup: =>
    if @running
      @running = false
      # TODO: Call external callback
      true
    false

  # Send as much data as the rate limit currently allows.
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
        @rateLimit.update chunk.length
        @relay.send chunk
        busy = true

      # websocket --> WebRTC
      if @webrtcIsReady() &&
         @client.bufferedAmount < @MAX_BUFFER &&
         @r2cSchedule.length > 0
        chunk = @r2cSchedule.shift()
        @rateLimit.update chunk.length
        @client.send chunk
        busy = true

    checkChunks() while busy  && !@rateLimit.isLimited()

    if @r2cSchedule.length > 0 || @c2rSchedule.length > 0 ||
       (@relayIsReady()  && @relay.bufferedAmount > 0) ||
       (@webrtcIsReady() && @client.bufferedAmount > 0)
      @flush_timeout_id = setTimeout @flush,  @rateLimit.when() * 1000
