###
A Coffeescript WebRTC snowflake proxy

Uses WebRTC from the client, and Websocket to the server.

Assume that the webrtc client plugin is always the offerer, in which case
this proxy must always act as the answerer.

TODO: More documentation
###

# General snowflake proxy constants.
# For websocket-specific constants, see websocket.coffee.
BROKER = 'snowflake-broker.bamsoftware.com'
RELAY =
  host: 'snowflake.bamsoftware.com'
  port: '443'
  # Original non-wss relay:
  # host: '192.81.135.242'
  # port: 9902
COOKIE_NAME = "snowflake-allow"

silenceNotifications = false
query = Query.parse(location)
DEBUG = Params.getBool(query, 'debug', false)

# Bytes per second. Set to undefined to disable limit.
DEFAULT_RATE_LIMIT = DEFAULT_RATE_LIMIT || undefined
MIN_RATE_LIMIT = 10 * 1024
RATE_LIMIT_HISTORY = 5.0
DEFAULT_BROKER_POLL_INTERVAL = 5.0 * 1000

MAX_NUM_CLIENTS = 1
CONNECTIONS_PER_CLIENT = 1

# TODO: Different ICE servers.
config = {
  iceServers: [
    { urls: ['stun:stun.l.google.com:19302'] }
  ]
}

# Janky state machine
MODE =
  INIT:              0
  WEBRTC_CONNECTING: 1
  WEBRTC_READY:      2

CONFIRMATION_MESSAGE = 'You\'re currently serving a Tor user via Snowflake.'

# Minimum viable snowflake for now - just 1 client.
class Snowflake

  relayAddr:  null
  proxyPairs: []
  rateLimit:  null
  state:      MODE.INIT
  retries:    0

  # Prepare the Snowflake with a Broker (to find clients) and optional UI.
  constructor: (@broker, @ui) ->
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
    @state = MODE.WEBRTC_CONNECTING
    for i in [1..CONNECTIONS_PER_CLIENT]
      @makeProxyPair @relayAddr
    log 'ProxyPair Slots: ' + @proxyPairs.length
    log 'Snowflake IDs: ' + (@proxyPairs.map (p) -> p.id).join ' | '
    @pollBroker()

  # Regularly poll Broker for clients to serve until this snowflake is
  # serving at capacity, at which point stop polling.
  pollBroker: ->
    # Temporary countdown. TODO: Simplify
    countdown = (msg, sec) =>
      dbg msg
      @ui?.setStatus msg + ' (Polling in ' + sec + ' seconds...)'
      sec--
      if sec >= 0
        setTimeout((-> countdown(msg, sec)), 1000)
      else
        findClients()
    # Poll broker for clients.
    findClients = =>
      pair = @nextAvailableProxyPair()
      if !pair
        log 'At client capacity.'
        # Do nothing until a new proxyPair is available.
        return
      msg = 'polling for client... '
      msg += '[retries: ' + @retries + ']' if @retries > 0
      @ui?.setStatus msg
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

snowflake = null

# Log to both console and UI if applicable.
# Requires that the snowflake and UI objects are hooked up in order to
# log to console.
log = (msg) ->
  console.log 'Snowflake: ' + msg
  snowflake?.ui?.log msg

dbg = (msg) -> log msg if DEBUG or snowflake.ui?.debug

snowflakeIsDisabled = ->
  cookies = Parse.cookie document.cookie
  # Do nothing if snowflake has not been opted in by user.
  if cookies[COOKIE_NAME] != '1'
    log 'Not opted-in. Please click the badge to change options.'
    return true
  # Also do nothing if running in Tor Browser.
  if mightBeTBB()
    log 'Will not run within Tor Browser.'
    return true
  return false


###
Entry point.
###
init = (isNode) ->
  # Hook up to the debug UI if available.
  ui = if isNode then null else new UI()
  silenceNotifications = Params.getBool(query, 'silent', false)
  broker = new Broker BROKER
  snowflake = new Snowflake broker, ui

  log '== snowflake proxy =='
  if snowflakeIsDisabled()
    # Do not activate the proxy if any number of conditions are true.
    log 'Currently not active.'
    return

  # Otherwise, begin setting up WebRTC and acting as a proxy.
  dbg 'Contacting Broker at ' + broker.url
  snowflake.setRelayAddr RELAY
  snowflake.beginWebRTC()

# Notification of closing tab with active proxy.
window.onbeforeunload = ->
  if !silenceNotifications && MODE.WEBRTC_READY == snowflake.state
    return CONFIRMATION_MESSAGE
  null

window.onunload = ->
  pair.close() for pair in snowflake.proxyPairs
  null

window.onload = init.bind null, false
