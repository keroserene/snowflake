###
A Coffeescript WebRTC snowflake proxy
Using Copy-paste signaling for now.

Uses WebRTC from the client, and websocket to the server.

Assume that the webrtc client plugin is always the offerer, in which case
this must always act as the answerer.
###
DEFAULT_WEBSOCKET = '192.81.135.242:9901'
DEFAULT_BROKER = 'https://snowflake-reg.appspot.com/'
COPY_PASTE_ENABLED = false
DEFAULT_PORTS =
  http:  80
  https: 443

DEBUG = false
if window && window.location
  query = Query.parse(window.location.search.substr(1))
  DEBUG = Params.getBool(query, 'debug', false)
# HEADLESS is true if we are running not in a browser with a DOM.
HEADLESS = 'undefined' == typeof(document)

# Bytes per second. Set to undefined to disable limit.
DEFAULT_RATE_LIMIT = DEFAULT_RATE_LIMIT || undefined
MIN_RATE_LIMIT = 10 * 1024
RATE_LIMIT_HISTORY = 5.0

MAX_NUM_CLIENTS = 1
CONNECTIONS_PER_CLIENT = 1

# TODO: Different ICE servers.
config = {
  iceServers: [
    { urls: ['stun:stun.l.google.com:19302'] }
  ]
}

# TODO: Implement
class Badge

# Janky state machine
MODE =
  INIT:              0
  WEBRTC_CONNECTING: 1
  WEBRTC_READY:      2

# Minimum viable snowflake for now - just 1 client.
class Snowflake

  relayAddr: null
  # TODO: Actually support multiple ProxyPairs. (makes more sense once meek-
  # signalling is ready)
  proxyPairs: []
  proxyPair: null

  rateLimit: null
  badge: null
  $badge: null
  state: MODE.INIT

  constructor: (@broker) ->
    if HEADLESS
      # No badge
    else if DEBUG
      @$badge = debug_div
    else
      @badge = new Badge()
      @$badgem = @badge.elem
    @$badge.setAttribute('id', 'snowflake-badge') if (@$badge)

    rateLimitBytes = undefined
    if 'off' != query['ratelimit']
      rateLimitBytes = Params.getByteCount(query, 'ratelimit',
                                           DEFAULT_RATE_LIMIT)
    if undefined == rateLimitBytes
      @rateLimit = new DummyRateLimit()
    else
      @rateLimit = new BucketRateLimit(rateLimitBytes * RATE_LIMIT_HISTORY,
                                       RATE_LIMIT_HISTORY)

  # TODO: Should fetch from broker later.
  setRelayAddr: (relayAddr) ->
    addr = Parse.address relayAddr
    if !addr
      log 'Invalid address spec.'
      return false
    @relayAddr = addr
    log 'Using ' + relayAddr + ' as Relay.'
    log 'Input offer from the snowflake client:' if COPY_PASTE_ENABLED
    return true

  # Initialize WebRTC PeerConnection
  beginWebRTC: ->
    @state = MODE.WEBRTC_CONNECTING
    for i in [1..CONNECTIONS_PER_CLIENT]
      @makeProxyPair @relayAddr
    @proxyPair = @proxyPairs[0]
    # Poll broker for clients.
    findClients = =>
      recv = broker.getClientOffer()
      recv.then((desc) =>
        offer = JSON.parse desc
        log 'Received:\n\n' + offer.sdp + '\n'
        @receiveOffer offer
      , (err) ->
        log err
        setTimeout(findClients, 1000)
      )
    findClients()

  # Receive an SDP offer from some client assigned by the Broker.
  receiveOffer: (desc) =>
    sdp = new RTCSessionDescription desc
    if @proxyPair.receiveWebRTCOffer sdp
      @sendAnswer() if 'offer' == sdp.type

  sendAnswer: =>
    next = (sdp) =>
      log 'webrtc: Answer ready.'
      @proxyPair.pc.setLocalDescription sdp
    promise = @proxyPair.pc.createAnswer next
    promise.then next if promise

  makeProxyPair: (relay) ->
    pair = new ProxyPair null, relay, @rateLimit
    @proxyPairs.push pair
    pair.onCleanup = (event) =>
      # Delete from the list of active proxy pairs.
      @proxyPairs.splice(@proxyPairs.indexOf(pair), 1)
      @badge.endProxy() if @badge
    try
      pair.begin()
    catch err
      log 'ERROR: ProxyPair exception while connecting.'
      log err
      return
    @badge.beginProxy if @badge

  cease: ->
    while @proxyPairs.length > 0
      @proxyPairs.pop().close()

  disable: ->
    log 'Disabling Snowflake.'
    @cease()
    @badge.disable() if @badge

  die: ->
    log 'Snowflake died.'
    @cease()
    @badge.die() if @badge

  # Close all existing ProxyPairs and begin finding new clients from scratch.
  reset: ->
    @cease()
    log '\nSnowflake resetting...'
    @beginWebRTC()

snowflake = null
broker = null

#
## -- DOM & Inputs -- #
#

# DOM elements references.
$msglog = null
$send = null
$input = null

Interface =
  # Local input from keyboard into message window.
  acceptInput: ->
    msg = $input.value
    if !COPY_PASTE_ENABLED
      log 'No input expected - Copy Paste Signalling disabled.'
    else switch snowflake.state
      when MODE.INIT
        # Set target relay.
        if !(snowflake.setRelayAddr msg)
          log 'Defaulting to websocket relay at ' + DEFAULT_WEBSOCKET
          snowflake.setRelayAddr DEFAULT_WEBSOCKET
      when MODE.WEBRTC_CONNECTING
        Signalling.receive msg
      when MODE.WEBRTC_READY
        log 'No input expected - WebRTC connected.'
      else
        log 'ERROR: ' + msg
    $input.value = ''
    $input.focus()

# Signalling channel - just tells user to copy paste to the peer.
# Eventually this should go over the broker.
Signalling =
  send: (msg) ->
    log '---- Please copy the below to peer ----\n'
    log JSON.stringify msg
    log '\n'

  receive: (msg) ->
    recv = ''
    try
      recv = JSON.parse msg
    catch e
      log 'Invalid JSON.'
      return
    desc = recv['sdp']
    if !desc
      log 'Invalid SDP.'
      return false
    snowflake.receiveOffer recv if desc

log = (msg) ->  # Log to the message window.
  console.log msg
  if $msglog                        # Scroll to latest
    $msglog.value += msg + '\n'
    $msglog.scrollTop = $msglog.scrollHeight

init = ->
  $msglog = document.getElementById('msglog')
  $msglog.value = ''

  $send = document.getElementById('send')
  $send.onclick = Interface.acceptInput

  $input = document.getElementById('input')
  $input.focus()
  $input.onkeydown = (e) -> $send.onclick() if 13 == e.keyCode  # enter

  log '== snowflake browser proxy =='
  broker = new Broker DEFAULT_BROKER
  snowflake = new Snowflake(broker)
  window.snowflake = snowflake
  if COPY_PASTE_ENABLED
    log 'Input desired relay address:'
  else
    snowflake.setRelayAddr DEFAULT_WEBSOCKET
    snowflake.beginWebRTC()

window.onload = init if window
