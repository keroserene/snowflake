###
WebrTC shims for multiple browsers.
###

if 'undefined' != typeof module && 'undefined' != typeof module.exports
  console.log 'not in browser.'
else
  window.PeerConnection = window.RTCPeerConnection ||
                          window.mozRTCPeerConnection ||
                          window.webkitRTCPeerConnection
  window.RTCIceCandidate = window.RTCIceCandidate || window.mozRTCIceCandidate
  window.RTCSessionDescription = window.RTCSessionDescription ||
                                 window.mozRTCSessionDescription

