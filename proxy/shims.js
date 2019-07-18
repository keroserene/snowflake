/* global module, require */

/*
WebRTC shims for multiple browsers.
*/

if (typeof module !== "undefined" && module !== null ? module.exports : void 0) {
  window = {};
  document = {
    getElementById: function() {
      return null;
    }
  };
  chrome = {};
  location = { search: '' };
  ({ URLSearchParams } = require('url'));
  if ((typeof TESTING === "undefined" || TESTING === null) || !TESTING) {
    webrtc = require('wrtc');
    PeerConnection = webrtc.RTCPeerConnection;
    IceCandidate = webrtc.RTCIceCandidate;
    SessionDescription = webrtc.RTCSessionDescription;
    WebSocket = require('ws');
    ({ XMLHttpRequest } = require('xmlhttprequest'));
  }
} else {
  PeerConnection = window.RTCPeerConnection || window.mozRTCPeerConnection || window.webkitRTCPeerConnection;
  IceCandidate = window.RTCIceCandidate || window.mozRTCIceCandidate;
  SessionDescription = window.RTCSessionDescription || window.mozRTCSessionDescription;
  WebSocket = window.WebSocket;
  XMLHttpRequest = window.XMLHttpRequest;
}
