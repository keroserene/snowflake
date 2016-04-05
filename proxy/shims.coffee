###
WebrTC shims for multiple browsers.
###

if typeof module isnt 'undefined' and module.exports
  window = {}
else
  window = this
  prop = Modernizr.prefixed 'RTCPeerConnection', window, false
  if not prop
    console.log 'webrtc feature not detected. shutting down'
    return;

  PeerConnection = window[prop]

  ### FIXME: push these upstream ###
  IceCandidate = window.RTCIceCandidate || window.mozRTCIceCandidate
  SessionDescription = window.RTCSessionDescription || window.mozRTCSessionDescription

location = if window.location then window.location.search.substr(1) else ""
