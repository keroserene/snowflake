###
A Coffeescript WebRTC snowflake proxy
Using Copy-paste signaling for now.

Uses WebRTC from the client, and websocket to the server.

Assume that the webrtc client plugin is always the offerer, in which case
this must always act as the answerer.
###

DEFAULT_WEBSOCKET = '192.81.135.242:9901'

if 'undefined' != typeof module && 'undefined' != typeof module.exports
  console.log 'not in browser.'
else
  window.PeerConnection = window.RTCPeerConnection ||
                          window.mozRTCPeerConnection ||
                          window.webkitRTCPeerConnection
  window.RTCIceCandidate = window.RTCIceCandidate || window.mozRTCIceCandidate
  window.RTCSessionDescription = window.RTCSessionDescription ||
                                 window.mozRTCSessionDescription

Query =
  ###
  Parse a URL query string or application/x-www-form-urlencoded body. The
  return type is an object mapping string keys to string values. By design,
  this function doesn't support multiple values for the same named parameter,
  for example 'a=1&a=2&a=3'; the first definition always wins. Returns null on
  error.

  Always decodes from UTF-8, not any other encoding.
  http://dev.w3.org/html5/spec/Overview.html#url-encoded-form-data
  ###
  parse: (qs) ->
    result = {}
    strings = []
    strings = qs.split '&' if qs
    return result if 0 == strings.length
    for string in strings
      j = string.indexOf '='
      if j == -1
        name = string
        value = ''
      else
        name = string.substr(0, j)
        value = string.substr(j + 1)
      name = decodeURIComponent(name.replace(/\+/g, ' '))
      value = decodeURIComponent(value.replace(/\+/g, ' '))
      result[name] = value if name not of result
    result

  # params is a list of (key, value) 2-tuples.
  buildString: (params) ->
    parts = []
    for param in params
      parts.push encodeURIComponent(param[0]) + '=' +
                 encodeURIComponent(param[1])
    parts.join '&'

Params =
  getBool: (query, param, defaultValue) ->
    val = query[param]
    return defaultValue if undefined == val
    return true if 'true' == val || '1' == val || '' == val
    return false if 'false' == val || '0' == val
    return null

  # Get an object value and parse it as a byte count. Example byte counts are
  # "100" and "1.3m". Returns default_val if param is not a key. Return null on
  # a parsing error.
  getByteCount: (query, param, defaultValue) ->
    spec = query[param]
    return defaultValue if undefined == spec
    parseByteCount spec

  # Parse a cookie data string (usually document.cookie). The return type is an
  # object mapping cookies names to values. Returns null on error.
  # http://www.w3.org/TR/DOM-Level-2-HTML/html.html#ID-8747038
  parseCookie: (cookies) ->
    result = {}
    strings = []
    strings = cookies.split ';' if cookies
    for string in strings
      j = string.indexOf '='
      return null if -1 == j
      name  = decodeURIComponent string.substr(0, j).trim()
      value = decodeURIComponent string.substr(j + 1).trim()
      result[name] = value if !(name in result)
    result

  # Parse an address in the form 'host:port'. Returns an Object with keys 'host'
  # (String) and 'port' (int). Returns null on error.
  parseAddress: (spec) ->
    m = null
    # IPv6 syntax.
    m = spec.match(/^\[([\0-9a-fA-F:.]+)\]:([0-9]+)$/) if !m
    # IPv4 syntax.
    m = spec.match(/^([0-9.]+):([0-9]+)$/) if !m
    return null if !m

    host = m[1]
    port = parseInt(m[2], 10)
    if isNaN(port) || port < 0 || port > 65535
      return null
    { host: host, port: port }

  # Parse a count of bytes. A suffix of "k", "m", or "g" (or uppercase)
  # does what you would think. Returns null on error.
  parseByteCount: (spec) ->
    UNITS = {
      k: 1024, m: 1024 * 1024, g: 1024 * 1024 * 1024
      K: 1024, M: 1024 * 1024, G: 1024 * 1024 * 1024
    }
    matches = spec.match /^(\d+(?:\.\d*)?)(\w*)$/
    return null if null == matches
    count = Number matches[1]
    return null if isNaN count
    if '' == matches[2]
      units = 1
    else
      units = UNITS[matches[2]]
      return null if null == units
    count * Number(units)

safe_repr = (s) -> SAFE_LOGGING ? '[scrubbed]' : JSON.stringify(s)

# HEADLESS is true if we are running not in a browser with a DOM.
DEBUG = false
if window && window.location
  query = Query.parse(window.location.search.substr(1))
  DEBUG = Params.getBool(query, 'debug', false)
HEADLESS = 'undefined' == typeof(document)

# Bytes per second. Set to undefined to disable limit.
DEFAULT_RATE_LIMIT = DEFAULT_RATE_LIMIT || undefined
MIN_RATE_LIMIT = 10 * 1024
RATE_LIMIT_HISTORY = 5.0

DEFAULT_PORTS =
  http:  80
  https: 443
# Build an escaped URL string from unescaped components. Only scheme and host
# are required. See RFC 3986, section 3.
buildUrl = (scheme, host, port, path, params) ->
  parts = []
  parts.push(encodeURIComponent scheme)
  parts.push '://'

  # If it contains a colon but no square brackets, treat it as IPv6.
  if host.match(/:/) && !host.match(/[[\]]/)
    parts.push '['
    parts.push host
    parts.push ']'
  else
    parts.push(encodeURIComponent host)

  if undefined != port && DEFAULT_PORTS[scheme] != port
    parts.push ':'
    parts.push(encodeURIComponent port.toString())

  if undefined != path && '' != path
    if !path.match(/^\//)
      path = '/' + path
    ###
    Slash is significant so we must protect it from encodeURIComponent, while
    still encoding question mark and number sign. RFC 3986, section 3.3: 'The
    path is terminated by the first question mark ('?') or number sign ('#')
    character, or by the end of the URI. ... A path consists of a sequence of
    path segments separated by a slash ('/') character.'
    ###
    path = path.replace /[^\/]+/, (m) ->
      encodeURIComponent m
    parts.push path

  if undefined != params
    parts.push '?'
    parts.push Query.buildString params

  parts.join ''

makeWebsocket = (addr) ->
  url = buildUrl 'ws', addr.host, addr.port, '/'
  # if have_websocket_binary_frames()
  ws = new WebSocket url
  # else
    # ws = new WebSocket url 'base64'
  ###
  'User agents can use this as a hint for how to handle incoming binary data: if
  the attribute is set to 'blob', it is safe to spool it to disk, and if it is
  set to 'arraybuffer', it is likely more efficient to keep the data in memory.'
  ###
  ws.binaryType = 'arraybuffer'
  ws

class BucketRateLimit
  amount: 0.0
  lastUpdate: new Date()

  constructor: (@capacity, @time) ->

  age: ->
    now = new Date()
    delta = (now - @lastUpdate) / 1000.0
    @lastUpdate = now
    @amount -= delta * @capacity / @time
    @amount = 0.0 if @amount < 0.0

  update: (n) ->
    @age()
    @amount += n
    @amount <= @capacity

  # How many seconds in the future will the limit expire?
  when: ->
    age()
    (@amount - @capacity) / (@capacity / @time)

  isLimited: ->
    @age()
    @amount > @capacity

# A rate limiter that never limits.
class DummyRateLimit
  constructor: (@capacity, @time) ->
  update: (n) -> true
  when: -> 0.0
  isLimited: -> false


# TODO: Different ICE servers.
config = {
  iceServers: [
    { urls: ['stun:stun.l.google.com:19302'] }
  ]
}

# DOM elements
$chatlog = null
$send = null
$input = null

# TODO: Implement
class Badge

# Janky state machine
MODE =
  INIT:              0
  WEBRTC_CONNECTING: 1
  WEBRTC_READY:      2

# Minimum viable snowflake for now - just 1 client.
class Snowflake

  MAX_NUM_CLIENTS = 1
  CONNECTIONS_PER_CLIENT = 1

  relayAddr: null
  # TODO: Actually support multiple ProxyPairs. (makes more sense once meek-
  # signalling is ready)
  proxyPairs: []
  proxyPair: null

  rateLimit: null
  badge: null
  $badge: null
  state: MODE.INIT

  constructor: ->
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
      rateLimitBytes = Params.getByteCount(query, 'ratelimit', DEFAULT_RATE_LIMIT)
    if undefined == rateLimitBytes
      @rateLimit = new DummyRateLimit()
    else
      @rateLimit = new BucketRateLimit(rateLimitBytes * RATE_LIMIT_HISTORY,
                                       RATE_LIMIT_HISTORY)

  # TODO: User-supplied for now, but should fetch from facilitator later.
  setRelayAddr: (relayAddr) ->
    addr = Params.parseAddress relayAddr
    if !addr
      log 'Invalid address spec.'
      return false
    @relayAddr = addr
    log 'Using ' + relayAddr + ' as Relay.'
    @beginWebRTC()
    log 'Input offer from the snowflake client:'
    return true

  # Initialize WebRTC PeerConnection
  beginWebRTC: ->
    log 'Starting up Snowflake...\n'
    @state = MODE.WEBRTC_CONNECTING
    for i in [1..CONNECTIONS_PER_CLIENT]
      @makeProxyPair @relayAddr
    @proxyPair = @proxyPairs[0]

  # Receive an SDP offer from client plugin.
  receiveOffer: (desc) =>
    sdp = new RTCSessionDescription desc
    try
      err = @proxyPair.pc.setRemoteDescription sdp
    catch e
      log 'Invalid SDP message.'
      return false
    log('SDP ' + sdp.type + ' successfully received.')
    @sendAnswer() if 'offer' == sdp.type
    true

  sendAnswer: =>
    next = (sdp) =>
      log 'webrtc: Answer ready.'
      @proxyPair.pc.setLocalDescription sdp
    promise = @proxyPair.pc.createAnswer next
    promise.then next if promise

  # Poll facilitator when this snowflake can support more clients.
  proxyMain: ->
    if @proxyPairs.length >= @MAX_NUM_CLIENTS * @CONNECTIONS_PER_CLIENT
      setTimeout(@proxyMain, @facilitator_poll_interval * 1000)
      return
    params = [['r', '1']]
    params.push ['transport', 'websocket']
    params.push ['transport', 'webrtc']

  makeProxyPair: (relay) ->
    pair = new ProxyPair(null, relay, @rateLimit);
    @proxyPairs.push pair
    pair.onCleanup = (event) =>
      # Delete from the list of active proxy pairs.
      @proxyPairs.splice(@proxyPairs.indexOf(pair), 1)
      @badge.endProxy() if @badge
    try
      pair.connectClient()
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

###
Represents: client <-- webrtc --> snowflake <-- websocket --> relay
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

  connectClient: =>
    @pc = new PeerConnection config, {
      optional: [
        { DtlsSrtpKeyAgreement: true }
        { RtpDataChannels: false }
      ]}
    @pc.onicecandidate = (evt) =>
      # Browser sends a null candidate once the ICE gathering completes.
      # In this case, it makes sense to send one copy-paste blob.
      if null == evt.candidate
        # TODO: Use a promise.all to tell Snowflake about all offers at once,
        # once multiple proxypairs are supported.
        log 'Finished gathering ICE candidates.'
        Signalling.send @pc.localDescription
    # OnDataChannel triggered remotely from the client when connection succeeds.
    @pc.ondatachannel = (dc) =>
      console.log dc;
      channel = dc.channel
      log 'Data Channel established...'
      @prepareDataChannel channel
      @client = channel

  prepareDataChannel: (channel) =>
    channel.onopen = =>
      log 'Data channel opened!'
      snowflake.state = MODE.WEBRTC_READY
      # This is the point when the WebRTC datachannel is done, so the next step
      # is to establish websocket to the server.
      @connectRelay()
    channel.onclose = =>
      log 'Data channel closed.'
      @state = MODE.INIT;
      $chatlog.className = ''
    channel.onerror = =>
      log 'Data channel error!'
    channel.onmessage = @onClientToRelayMessage

  # Assumes WebRTC datachannel is connected.
  connectRelay: =>
    log 'Connecting to relay...'
    @relay = makeWebsocket @relayAddr
    @relay.label = 'websocket-relay'
    @relay.onopen = =>
      log '\nRelay ' + @relay.label + ' connected!'
    @relay.onclose = @onClose
    @relay.onerror = @onError
    @relay.onmessage = @onRelayToClientMessage

  # WebRTC --> websocket
  onClientToRelayMessage: (msg) =>
    line = recv = msg.data
    console.log msg
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
    @client.close() if !(isClosed @client)
    @relay.close() if !(isClosed @relay)

  maybeCleanup: =>
    if @running && @isClosed @relay
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
      if @relayIsReady() && @relay.bufferedAmount < @MAX_BUFFER && @c2rSchedule.length > 0
        chunk = @c2rSchedule.shift()
        @rateLimit.update chunk.length
        @relay.send chunk
        busy = true
      # websocket --> WebRTC
      if @webrtcIsReady() && @client.bufferedAmount < @MAX_BUFFER && @r2cSchedule.length > 0
        chunk = @r2cSchedule.shift()
        @rateLimit.update chunk.length
        @client.send chunk
        busy = true
    checkChunks() while busy  && !@rateLimit.isLimited()

    if @r2cSchedule.length > 0 || @c2rSchedule.length > 0 || (@relayIsReady() && @relay.bufferedAmount > 0) || (@webrtcIsReady() && @client.bufferedAmount > 0)
      @flush_timeout_id = setTimeout @flush,  @rateLimit.when() * 1000

#
## -- DOM & Input Functionality -- ##
#
snowflake = null

welcome = ->
  log '== snowflake browser proxy =='
  log 'Input desired relay address:'

# Log to the message window.
log = (msg) ->
  console.log msg
  # Scroll to latest
  if $chatlog
    $chatlog.value += msg + '\n'
    $chatlog.scrollTop = $chatlog.scrollHeight

Interface =
  # Local input from keyboard into message window.
  acceptInput: ->
    msg = $input.value
    switch snowflake.state
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
# Eventually this should go over the facilitator.
Signalling =
  send: (msg) ->
    log '---- Please copy the below to peer ----\n'
    log JSON.stringify(msg)
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

init = ->

  $chatlog = document.getElementById('chatlog')
  $chatlog.value = ''

  $send = document.getElementById('send')
  $send.onclick = Interface.acceptInput

  $input = document.getElementById('input')
  $input.focus()
  $input.onkeydown = (e) =>
    if 13 == e.keyCode  # enter
      $send.onclick()
  snowflake = new Snowflake()
  window.snowflake = snowflake
  welcome()

window.onload = init if window
