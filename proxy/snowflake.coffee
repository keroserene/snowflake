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

# Janky state machine
MODE =
  INIT:       0
  CONNECTING: 1
  CHAT:       2
currentMode = MODE.INIT


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

# Minimum viable snowflake for now - just 1 client.
class Snowflake

  rateLimit: 0
  proxyPairs: null
  badge: null
  $badge: null
  MAX_NUM_CLIENTS = 1

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

  start: ->
    log "Starting up Snowflake..."

  # Poll facilitator when this snowflake can support more clients.
  proxyMain = ->
    if @proxyPairs.length >= @MAX_NUM_CLIENTS * CONNECTIONS_PER_CLIENT
      setTimeout(@proxyMain, @facilitator_poll_interval * 1000)
      return

    params = [["r", "1"]]
    params.push ["transport", "websocket"]
    params.push ["transport", "webrtc"]

  beginProxy = (client, relay) ->
    for i in [0..CONNECTIONS_PER_CLIENT]
      makeProxyPair(client, relay)

  makeProxyPair = (client, relay) ->
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

  cease = ->
    # @start = null
    # @proxyMain = null
    # @make_proxy_pair = function(client_addr, relay_addr) { };
    while @proxyPairs.length > 0
      @proxyPairs.pop().close()

  disable = ->
    log "Disabling Snowflake."
    @cease()
    @badge.disable() if @badge

  die = ->
    log "Snowflake died."
    @cease()
    @badge.die() if @badge


# TODO: Implement
class ProxyPair


#
## -- DOM & Input Functionality -- ##
#
snowflake = new Snowflake()

welcome = ->
  log "== snowflake browser proxy =="
  log "Input offer from the snowflake client:"

# Log to the message window.
log = (msg) ->
  $chatlog.value += msg + "\n"
  console.log msg
  # Scroll to latest
  $chatlog.scrollTop = $chatlog.scrollHeight

# Local input from keyboard into message window.
acceptInput = () ->
  msg = $input.value
  switch currentMode
    when MODE.INIT
      if msg.startsWith("start")
        start(true)
      else
        Signalling.receive msg
    when MODE.CONNECTING
      Signalling.receive msg
    when MODE.CHAT
      data = msg
      log(data)
      channel.send(data)
    else
      log("ERROR: " + msg)
  $input.value = "";
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
    # Begin as answerer if peerconnection doesn't exist yet.
    snowflake.start false if !pc
    desc = recv['sdp']
    ice = recv['candidate']
    if !desc && ! ice
      log "Invalid SDP."
      return false
    receiveDescription recv if desc
    receiveICE recv         if ice

init = ->
  $chatlog = document.getElementById('chatlog')
  $chatlog.value = ""

  $send = document.getElementById('send')
  $send.onclick = acceptInput

  $input = document.getElementById('input')
  $input.focus()
  $input.onkeydown = (e) =>
    if 13 == e.keyCode  # enter
      $send.onclick()
  welcome()

window.onload = init
