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

  prop = Modernizr.prefixed 'RTCPeerConnection', window, false
  if not prop
    console.log 'webrtc feature not detected. shutting down'
    return

  PeerConnection = window[prop]

  ### FIXME: push these upstream ###
  IceCandidate = window.RTCIceCandidate || window.mozRTCIceCandidate
  SessionDescription = window.RTCSessionDescription ||
    window.mozRTCSessionDescription

  WebSocket = window.WebSocket
  XMLHttpRequest = window.XMLHttpRequest
