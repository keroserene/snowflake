###
WebRTC shims for multiple browsers.
###

if module?.exports
  window = {}
  location = ''

  if not TESTING? or not TESTING
    webrtc = require 'wrtc'

    PeerConnection = webrtc.RTCPeerConnection
    IceCandidate = webrtc.RTCIceCandidate
    SessionDescription = webrtc.RTCSessionDescription

    WebSocket = require 'ws'
    { XMLHttpRequest } = require 'xmlhttprequest'

    process.nextTick () -> init true

else
  window = this
  location = window.location.search.substr(1)

  PeerConnection = window.RTCPeerConnection || window.mozRTCPeerConnection ||
    window.webkitRTCPeerConnection
  IceCandidate = window.RTCIceCandidate || window.mozRTCIceCandidate
  SessionDescription = window.RTCSessionDescription ||
    window.mozRTCSessionDescription

  if typeof PeerConnection isnt 'function'
    console.log 'webrtc feature not detected. shutting down'
    return

  WebSocket = window.WebSocket
  XMLHttpRequest = window.XMLHttpRequest
