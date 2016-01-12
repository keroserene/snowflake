###
A Coffeescript WebRTC snowflake proxy
Using Copy-paste signaling for now.

Uses WebRTC from the client, and websocket to the server.

Assume that the webrtc client plugin is always the offerer, in which case
this must always act as the answerer.

TODO(keroserene): Complete the websocket + webrtc ProxyPair
###

if 'undefined' != typeof module && 'undefined' != typeof module.exports
  console.log 'not in browser.'
else
  window.PeerConnection = window.RTCPeerConnection ||
                          window.mozRTCPeerConnection ||
                          window.webkitRTCPeerConnection
  window.RTCIceCandidate = window.RTCIceCandidate || window.mozRTCIceCandidate;
  window.RTCSessionDescription = window.RTCSessionDescription ||
                                 window.mozRTCSessionDescription

Query =
  ###
  Parse a URL query string or application/x-www-form-urlencoded body. The
  return type is an object mapping string keys to string values. By design,
  this function doesn't support multiple values for the same named parameter,
  for example "a=1&a=2&a=3"; the first definition always wins. Returns null on
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
    return true if "true" == val || "1" == val || "" == val
    return false if "false" == val || "0" == val
    return null


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

  # Parse an address in the form "host:port". Returns an Object with keys "host"
  # (String) and "port" (int). Returns null on error.
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

# repr = (x) ->
  # return 'null' if null == x
  # return 'undefined' if 'undefined' == typeof x
  # if 'object' == typeof x
    # elems = []
    # for k in x
      # elems.push(maybe_quote(k) + ': ' + repr(x[k]));
    # return "{ " + elems.join(", ") + " }";
  # } else if (typeof x === "string") {
    # return quote(x);
  # } else {
    # return x.toString();
# safe_repr = (s) -> SAFE_LOGGING ? "[scrubbed]" : repr(s)
safe_repr = (s) -> SAFE_LOGGING ? "[scrubbed]" : JSON.stringify(s)

# HEADLESS is true if we are running not in a browser with a DOM.
DEBUG = false
if window && window.location
  query = Query.parse(window.location.search.substr(1))
  DEBUG = Params.getBool(query, "debug", false)
HEADLESS = "undefined" == typeof(document)

# TODO: Different ICE servers.
config = {
  iceServers: [
    { urls: ["stun:stun.l.google.com:19302"] }
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

  # PeerConnection
  pc: null
  rateLimit: 0
  proxyPairs: []
  relayAddr: null
  badge: null
  $badge: null
  MAX_NUM_CLIENTS = 1
  CONNECTIONS_PER_CLIENT = 1
  state: MODE.INIT

  constructor: ->
    if HEADLESS
      # No badge
    else if DEBUG
      @$badge = debug_div
    else
      @badge = new Badge()
      @$badgem = @badge.elem
    @$badge.setAttribute("id", "snowflake-badge") if (@$badge)

  # TODO: User-supplied for now, but should fetch from facilitator later.
  setRelayAddr: (relayAddr) ->
    addr = Params.parseAddress relayAddr
    if !addr
      log 'Invalid address spec. Try again.'
      return false
    @relayAddr = addr
    log 'Using ' + relayAddr + ' as Relay.'
    log "Input offer from the snowflake client:"
    @beginWebRTC()
    return true

  # Initialize WebRTC PeerConnection
  beginWebRTC: ->
    log "Starting up Snowflake..."
    @state = MODE.WEBRTC_CONNECTING

    @pc = new PeerConnection(config, {
      optional: [
        { DtlsSrtpKeyAgreement: true }
        { RtpDataChannels: false }
      ]
    })

    @pc.onicecandidate = (evt) =>
      # Browser sends a null candidate once the ICE gathering completes.
      # In this case, it makes sense to send one copy-paste blob.
      if null == evt.candidate
        log "Finished gathering ICE candidates."
        Signalling.send @pc.localDescription

    # OnDataChannel triggered remotely from the client when connection succeeds.
    @pc.ondatachannel = (dc) =>
      console.log dc;
      channel = dc.channel
      log "Data Channel established..."
      @prepareDataChannel channel

  prepareDataChannel: (channel) ->
    channel.onopen = =>
      log "Data channel opened!"
      @state = MODE.WEBRTC_READY
      # This is the point when the WebRTC datachannel is done, so the next step
      # is to establish websocket to the server.
      @beginProxy(null, @relayAddr)
    channel.onclose = =>
      log "Data channel closed."
      @state = MODE.INIT;
      $chatlog.className = ""
    channel.onerror = =>
      log "Data channel error!"
    channel.onmessage = (msg) =>
      line = recv = msg.data
      console.log(msg);
      # Go sends only raw bytes...
      if "[object ArrayBuffer]" == recv.toString()
        bytes = new Uint8Array recv
        line = String.fromCharCode.apply(null, bytes)
      line = line.trim()
      log "data: " + line

  # Receive an SDP offer from client plugin.
  receiveOffer: (desc) =>
    sdp = new RTCSessionDescription desc
    try
      err = @pc.setRemoteDescription sdp
    catch e
      log "Invalid SDP message."
      return false
    log("SDP " + sdp.type + " successfully received.")
    @sendAnswer() if 'offer' == sdp.type
    true

  sendAnswer: =>
    next = (sdp) =>
      log "webrtc: Answer ready."
      @pc.setLocalDescription sdp
    promise = @pc.createAnswer next
    promise.then next if promise

  # Poll facilitator when this snowflake can support more clients.
  proxyMain: ->
    if @proxyPairs.length >= @MAX_NUM_CLIENTS * @CONNECTIONS_PER_CLIENT
      setTimeout(@proxyMain, @facilitator_poll_interval * 1000)
      return

    params = [["r", "1"]]
    params.push ["transport", "websocket"]
    params.push ["transport", "webrtc"]

  beginProxy: (client, relay) ->
    for i in [1..CONNECTIONS_PER_CLIENT]
      @makeProxyPair client, relay

  makeProxyPair: (client, relay) ->
    pair = new ProxyPair(client, relay, @rate_limit);
    @proxyPairs.push pair
    pair.onCleanup = (event) =>
      # Delete from the list of active proxy pairs.
      @proxyPairs.splice(@proxy_pairs.indexOf(pair), 1)
      @badge.endProxy() if @badge
    try
      pair.connectRelay()
    catch err
      log 'ERROR: ProxyPair exception while connecting.'
      log err
      return
    @badge.beginProxy if @badge

  cease: ->
    while @proxyPairs.length > 0
      @proxyPairs.pop().close()

  disable: ->
    log "Disabling Snowflake."
    @cease()
    @badge.disable() if @badge

  die: ->
    log "Snowflake died."
    @cease()
    @badge.die() if @badge


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
    still encoding question mark and number sign. RFC 3986, section 3.3: "The
    path is terminated by the first question mark ('?') or number sign ('#')
    character, or by the end of the URI. ... A path consists of a sequence of
    path segments separated by a slash ('/') character."
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
  "User agents can use this as a hint for how to handle incoming binary data: if
  the attribute is set to 'blob', it is safe to spool it to disk, and if it is
  set to 'arraybuffer', it is likely more efficient to keep the data in memory."
  ###
  ws.binaryType = 'arraybuffer'
  ws



# TODO: Implement
class ProxyPair

  constructor: (@clientAddr, @relayAddr, @rateLimit) ->

  # Assumes WebRTC part is already connected.
  # TODO: Put the webrtc stuff in ProxyPair, so that multiple webrtc connections
  # can be established.
  connectRelay: ->
    log "Snowflake: connecting to relay"

    @relay = makeWebsocket(@relayAddr);
    @relay.label = 'Relay'
    @relay.onopen = =>
      log "Snowflake: " + ws.label + "connected"
    @relay.onclose = @onClose
    @relay.onerror = @onError
    @relay.onmessage = @onRelayToClientMessage

  onClientToRelayMessage: (event) ->
    @c2r_schedule.push event.data
    @flush()

  onRelayToClientMessage: (event) ->
    @r2c_schedule.push event.data
    @flush()

  onClose: (event) ->
    ws = event.target
    log(ws.label + ': closed.')
    @flush()
    @maybeCleanup()

  onError: (event) ->
    ws = event.target
    log ws.label + ': error.'
    this.close();
    # we can't rely on onclose_callback to cleanup, since one common error
    # case is when the client fails to connect and the relay never starts.
    # in that case close() is a NOP and onclose_callback is never called.
    @maybeCleanup()

  isOpen:   (ws) -> undefined != ws && WebSocket.OPEN   == ws.readyState
  isClosed: (ws) -> undefined == ws || WebSocket.CLOSED == ws.readyState

  maybeCleanup: ->
    if @running && @isClosed(client) && @isClosed @relay 
      @running = false
      @cleanup_callback()
      true
    false

  # Send as much data as the rate limit currently allows.
  flush: ->
  ###
    clearTimeout @flush_timeout_id if @flush_timeout_id
    @flush_timeout_id = null
    busy = true
    checkChunks = ->
      busy = false
      # if @isOpen @clientthis.client_s) &&
          # this.client_s.bufferedAmount < MAX_BUFFER &&
          # this.r2c_schedule.length > 0) {
        # chunk = this.r2c_schedule.shift();
        # this.rate_limit.update(chunk.length);
        # this.client_s.send(chunk);
        # busy = true;
      #
      if @isOpen @relay &&
        @relay.bufferedAmount < MAX_BUFFER &&
        @c2r_schedule.length > 0
        chunk = @c2r_schedule.shift()
        @rate_limit.update chunk.length
        @relay.send chunk
        busy = true
    checkChunks() while busy && !@rate_limit.is_limited()

    if @isClosed @relay &&
        # !isClosed(this.client_s) &&
        # @client_s.bufferedAmount === 0 &&
        @r2c_schedule.length == 0
      # log("Client: closing.");
      # this.client_s.close();
      # if (is_closed(this.client_s) &&
        # !is_closed(this.relay_s) &&
        # this.relay_s.bufferedAmount === 0 &&
        # this.c2r_schedule.length === 0) {
      # log("Relay: closing.");
      # this.relay_s.close();
    # }


    while busy && !@rate_limit.is_limited()

    if this.r2c_schedule.length > 0 ||
      (@isOpen(@client) && @client.bufferedAmount > 0) ||
      @c2r_schedule.length > 0 ||
      (@isOpen(@relay) && @relay.bufferedAmount > 0)
      @flush_timeout_id = setTimeout @flush, @rate_limit.when() * 1000
  ###
#
## -- DOM & Input Functionality -- ##
#
snowflake = null

welcome = ->
  log "== snowflake browser proxy =="
  log "Input desired relay address:"

# Log to the message window.
log = (msg) ->
  $chatlog.value += msg + "\n"
  console.log msg
  # Scroll to latest
  $chatlog.scrollTop = $chatlog.scrollHeight

Interface =
  # Local input from keyboard into message window.
  acceptInput: ->
    msg = $input.value
    switch snowflake.state
      when MODE.INIT
        # Set target relay.
        snowflake.setRelayAddr msg
      when MODE.WEBRTC_CONNECTING
        Signalling.receive msg
      when MODE.WEBRTC_READY
        log "No input expected - WebRTC connected."
        # data = msg
        # log(data)
        # channel.send(data)
      else
        log "ERROR: " + msg
    $input.value = ""
    $input.focus()

# Signalling channel - just tells user to copy paste to the peer.
# Eventually this should go over the facilitator.
Signalling =
  send: (msg) ->
    log "---- Please copy the below to peer ----\n"
    log JSON.stringify(msg)
    log "\n"

  receive: (msg) ->
    recv = ""
    try
      recv = JSON.parse msg
    catch e
      log "Invalid JSON."
      return
    desc = recv['sdp']
    if !desc
      log "Invalid SDP."
      return false
    snowflake.receiveOffer recv if desc

init = ->

  $chatlog = document.getElementById('chatlog')
  $chatlog.value = ""

  $send = document.getElementById('send')
  $send.onclick = Interface.acceptInput

  $input = document.getElementById('input')
  $input.focus()
  $input.onkeydown = (e) =>
    if 13 == e.keyCode  # enter
      $send.onclick()
  snowflake = new Snowflake()
  welcome()

window.onload = init if window
