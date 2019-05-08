###
A Coffeescript WebRTC snowflake proxy

Uses WebRTC from the client, and Websocket to the server.

Assume that the webrtc client plugin is always the offerer, in which case
this proxy must always act as the answerer.

TODO: More documentation
###

# Minimum viable snowflake for now - just 1 client.
class Snowflake
  relayAddr:  null
  rateLimit:  null
  retries:    0

  # Janky state machine
  @MODE =
    INIT:              0
    WEBRTC_CONNECTING: 1
    WEBRTC_READY:      2

  # Prepare the Snowflake with a Broker (to find clients) and optional UI.
  constructor: (@broker, @ui) ->
    @state = Snowflake.MODE.INIT
    @proxyPairs = []

    rateLimitBytes = undefined
    if 'off' != query['ratelimit']
      rateLimitBytes = Params.getByteCount(query, 'ratelimit',
                                           DEFAULT_RATE_LIMIT)
    if undefined == rateLimitBytes
      @rateLimit = new DummyRateLimit()
    else
      @rateLimit = new BucketRateLimit(rateLimitBytes * RATE_LIMIT_HISTORY,
                                       RATE_LIMIT_HISTORY)
    @retries = 0

  # Set the target relay address spec, which is expected to be websocket.
  # TODO: Should potentially fetch the target from broker later, or modify
  # entirely for the Tor-independent version.
  setRelayAddr: (relayAddr) ->
    @relayAddr = relayAddr
    log 'Using ' + relayAddr.host + ':' + relayAddr.port + ' as Relay.'
    return true

  # Initialize WebRTC PeerConnection, which requires beginning the signalling
  # process. |pollBroker| automatically arranges signalling.
  beginWebRTC: ->
    @state = Snowflake.MODE.WEBRTC_CONNECTING
    for i in [1..CONNECTIONS_PER_CLIENT]
      @makeProxyPair @relayAddr
    log 'ProxyPair Slots: ' + @proxyPairs.length
    log 'Snowflake IDs: ' + (@proxyPairs.map (p) -> p.id).join ' | '
    @pollBroker()

  # Regularly poll Broker for clients to serve until this snowflake is
  # serving at capacity, at which point stop polling.
  pollBroker: ->
    # Temporary countdown. TODO: Simplify
    countdown = (msg, sec, skip) =>
      if not skip then dbg msg
      if sec > 0
        @ui.setStatus msg + ' (Polling in ' + sec + ' seconds...)'
        sec--
        setTimeout((-> countdown(msg, sec, true)), 1000)
      else
        @ui.setStatus msg
        findClients()
    # Poll broker for clients.
    findClients = =>
      pair = @nextAvailableProxyPair()
      if !pair
        log 'At client capacity.'
        # Do nothing until a new proxyPair is available.
        return
      msg = 'Polling for client ... '
      msg += '[retries: ' + @retries + ']' if @retries > 0
      @ui.setStatus msg
      recv = @broker.getClientOffer pair.id
      recv.then (desc) =>
        @receiveOffer pair, desc
        countdown('Serving 1 new client.', DEFAULT_BROKER_POLL_INTERVAL / 1000)
      , (err) ->
        countdown(err, DEFAULT_BROKER_POLL_INTERVAL / 1000)
      @retries++

    findClients()

  # Returns the first ProxyPair that's available to connect.
  nextAvailableProxyPair: ->
    return @proxyPairs.find (pp, i, arr) -> return !pp.active

  # Receive an SDP offer from some client assigned by the Broker,
  # |pair| - an available ProxyPair.
  receiveOffer: (pair, desc) =>
    console.assert !pair.active
    try
      offer = JSON.parse desc
      dbg 'Received:\n\n' + offer.sdp + '\n'
      sdp = new SessionDescription offer
      @sendAnswer pair if pair.receiveWebRTCOffer sdp
    catch e
      log 'ERROR: Unable to receive Offer: ' + e

  sendAnswer: (pair) ->
    next = (sdp) ->
      dbg 'webrtc: Answer ready.'
      pair.pc.setLocalDescription sdp
    fail = ->
      dbg 'webrtc: Failed to create Answer'
    pair.pc.createAnswer()
    .then next
    .catch fail

  makeProxyPair: (relay) ->
    pair = new ProxyPair relay, @rateLimit
    @proxyPairs.push pair
    pair.onCleanup = (event) =>
      # Delete from the list of active proxy pairs.
      @proxyPairs.splice(@proxyPairs.indexOf(pair), 1)
      @pollBroker()
    pair.begin()

  # Stop all proxypairs.
  cease: ->
    while @proxyPairs.length > 0
      @proxyPairs.pop().close()

  disable: ->
    log 'Disabling Snowflake.'
    @cease()

  die: ->
    log 'Snowflake died.'
    @cease()

  # Close all existing ProxyPairs and begin finding new clients from scratch.
  reset: ->
    @cease()
    log 'Snowflake resetting...'
    @retries = 0
    @beginWebRTC()
