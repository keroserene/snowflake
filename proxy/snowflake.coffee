###
A Coffeescript WebRTC snowflake proxy
Using Copy-paste signaling for now.
###

# Janky state machine
MODE =
  INIT:       0
  CONNECTING: 1
  CHAT:       2
currentMode = MODE.INIT

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
                        window.mozRTCPeerConnection || window.webkitRTCPeerConnection
window.RTCIceCandidate = window.RTCIceCandidate || window.mozRTCIceCandidate;
window.RTCSessionDescription = window.RTCSessionDescription || window.mozRTCSessionDescription

class Snowflake

class ProxyPair

#
## -- DOM & Input Functionality -- ##
#

welcome = ->
  log "== snowflake JS proxy =="
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
    start false if !pc
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
