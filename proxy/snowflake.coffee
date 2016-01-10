###
A Coffeescript WebRTC snowflake proxy
Using Copy-paste signaling for now.

Uses WebRTC from the client, and websocket to the server.

Assume that the webrtc client plugin is always the offerer, in which case
this must always act as the answerer.
###

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
    for i in [1..strings.length]
      string = strings[i]
      j = string.indexOf '='
      if j == -1
        name = string
        value = ''
      else
        name = string.substr(0, j)
        value = string.substr(j + 1)
      name = decodeURIComponent(name.replace(/\+/g, ' '))
      value = decodeURIComponent(value.replace(/\+/g, ' '))
      result[name] = value if !(name in result)
    result

Params =
  getBool: (query, param, defaultValue) ->
    val = query[param]
    return defaultValue if undefined == val
    return true if "true" == val || "1" == val || "" == val
    return false if "false" == val || "0" == val
    return null

# HEADLESS is true if we are running not in a browser with a DOM.
query = Query.parse(window.location.search.substr(1))
HEADLESS = "undefined" == typeof(document)
DEBUG = Params.getBool(query, "debug", false)

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

window.PeerConnection = window.RTCPeerConnection ||
                        window.mozRTCPeerConnection ||
                        window.webkitRTCPeerConnection
window.RTCIceCandidate = window.RTCIceCandidate || window.mozRTCIceCandidate;
window.RTCSessionDescription = window.RTCSessionDescription ||
                               window.mozRTCSessionDescription

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
  proxyPairs: null
  badge: null
  $badge: null
  MAX_NUM_CLIENTS = 1
  state: MODE.INIT

  constructor: ->
    if HEADLESS
      # No badge
    else if DEBUG
      @$badge = debug_div
    else
      @badge = new Badge()
      @$badgem = @badge.elem
    if (@$badge)
      @$badge.setAttribute("id", "snowflake-badge")

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
      # TODO: Prepare ProxyPair onw.
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
    @sendAnswer() if "offer" == sdp.type
    true

  sendAnswer: =>
    next = (sdp) =>
      log "webrtc: Answer ready."
      @pc.setLocalDescription sdp
    promise = @pc.createAnswer next
    promise.then next if promise

  # Poll facilitator when this snowflake can support more clients.
  proxyMain: ->
    if @proxyPairs.length >= @MAX_NUM_CLIENTS * CONNECTIONS_PER_CLIENT
      setTimeout(@proxyMain, @facilitator_poll_interval * 1000)
      return

    params = [["r", "1"]]
    params.push ["transport", "websocket"]
    params.push ["transport", "webrtc"]

  beginProxy: (client, relay) ->
    for i in [0..CONNECTIONS_PER_CLIENT]
      makeProxyPair(client, relay)

  makeProxyPair: (client, relay) ->
    pair = new ProxyPair(client, relay, @rate_limit);
    @proxyPairs.push pair
    pair.onCleanup = (event) =>
      # Delete from the list of active proxy pairs.
      @proxyPairs.splice(@proxy_pairs.indexOf(pair), 1)
      @badge.endProxy() if @badge
    try
      proxy_pair.connect()
    catch err
      log "ProxyPair: exception while connecting: " + safe_repr(err.message) + "."
      return
    @badge.beginProxy if @badge

  cease: ->
    # @start = null
    # @proxyMain = null
    # @make_proxy_pair = function(client_addr, relay_addr) { };
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


# TODO: Implement
class ProxyPair


#
## -- DOM & Input Functionality -- ##
#
snowflake = null

welcome = ->
  log "== snowflake browser proxy =="
  log "Input offer from the snowflake client:"

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
  snowflake.beginWebRTC()
  welcome()

window.onload = init
