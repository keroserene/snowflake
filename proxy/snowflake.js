/*
JS WebRTC proxy
Copy-paste signaling.
*/

// DOM elements
var $chatlog, $input, $send, $name;

var config = {
  iceServers: [
    { urls: ["stun:stun.l.google.com:19302"] }
  ]
}

// Chrome / Firefox compatibility
window.PeerConnection = window.RTCPeerConnection ||
                        window.mozRTCPeerConnection || window.webkitRTCPeerConnection;
window.RTCIceCandidate = window.RTCIceCandidate || window.mozRTCIceCandidate;
window.RTCSessionDescription = window.RTCSessionDescription || window.mozRTCSessionDescription;

var pc;  // PeerConnection
var offer, answer;
var channel;

// Janky state machine
var MODE = {
  INIT:       0,
  CONNECTING: 1,
  CHAT:       2
}
var currentMode = MODE.INIT;
var CONNECTIONS_PER_CLIENT = 2;

// Signalling channel - just tells user to copy paste to the peer.
// Eventually this should go over the facilitator.
var Signalling = {
  send: function(msg) {
    log("---- Please copy the below to peer ----\n");
    log(JSON.stringify(msg));
    log("\n");
  },
  receive: function(msg) {
    var recv;
    try {
      recv = JSON.parse(msg);
    } catch(e) {
      log("Invalid JSON.");
      return;
    }
    if (!pc) {
      start(false);
    }
    var desc = recv['sdp']
    var ice = recv['candidate']
    if (!desc && ! ice) {
      log("Invalid SDP.");
      return false;
    }
    if (desc) { receiveDescription(recv); }
    if (ice) { receiveICE(recv); }
  }
}

function welcome() {
  log("== snowflake JS proxy ==");
  log("Input offer from the snowflake client:");
}

function start(initiator) {
  log("Starting up RTCPeerConnection...");
  pc = new PeerConnection(config, {
    optional: [
      { DtlsSrtpKeyAgreement: true },
      { RtpDataChannels: false },
    ],
  });
  pc.onicecandidate = function(evt) {
    var candidate = evt.candidate;
    // Chrome sends a null candidate once the ICE gathering phase completes.
    // In this case, it makes sense to send one copy-paste blob.
    if (null == candidate) {
      log("Finished gathering ICE candidates.");
      Signalling.send(pc.localDescription);
      return;
    }
  }
  pc.onnegotiationneeded = function() {
    sendOffer();
  }
  pc.ondatachannel = function(dc) {
    console.log(dc);
    channel = dc.channel;
    log("Data Channel established... ");
    prepareDataChannel(channel);
  }

  // Creating the first data channel triggers ICE negotiation.
  if (initiator) {
    channel = pc.createDataChannel("test");
    prepareDataChannel(channel);
  }
}

// Local input from keyboard into chat window.
function acceptInput(is) {
  var msg = $input.value;
  switch (currentMode) {
    case MODE.INIT:
      if (msg.startsWith("start")) {
        start(true);
      } else {
        Signalling.receive(msg);
      }
      break;
    case MODE.CONNECTING:
      Signalling.receive(msg);
      break;
    case MODE.CHAT:
      var data = msg;
      log(data);
      channel.send(data);
      break;
    default:
      log("ERROR: " + msg);
  }
  $input.value = "";
  $input.focus();
}

// Chrome uses callbacks while Firefox uses promises.
// Need to support both - same for createAnswer below.
function sendOffer() {
  var next = function(sdp) {
    log("webrtc: Created Offer");
    offer = sdp;
    pc.setLocalDescription(sdp);
  }
  var promise = pc.createOffer(next);
  if (promise) {
    promise.then(next);
  }
}

function sendAnswer() {
  var next = function (sdp) {
    log("webrtc: Created Answer");
    answer = sdp;
    pc.setLocalDescription(sdp)
  }
  var promise = pc.createAnswer(next);
  if (promise) {
    promise.then(next);
  }
}

function receiveDescription(desc) {
  var sdp = new RTCSessionDescription(desc);
  try {
    err = pc.setRemoteDescription(sdp);
  } catch (e) {
    log("Invalid SDP message.");
    return false;
  }
  log("SDP " + sdp.type + " successfully received.");
  if ("offer" == sdp.type) {
    sendAnswer();
  }
  return true;
}

function receiveICE(ice) {
  var candidate = new RTCIceCandidate(ice);
  try {
    pc.addIceCandidate(candidate);
  } catch (e) {
    log("Could not add ICE candidate.");
    return;
  }
  log("ICE candidate successfully received: " + ice.candidate);
}

function waitForSignals() {
  currentMode = MODE.CONNECTING;
}

function prepareDataChannel(channel) {
  channel.onopen = function() {
    log("Data channel opened!");
    startChat();
  }
  channel.onclose = function() {
    log("Data channel closed.");
    currentMode = MODE.INIT;
    $chatlog.className = "";
    log("------- chat disabled -------");
  }
  channel.onerror = function() {
    log("Data channel error!!");
  }
  channel.onmessage = function(msg) {
    var recv = msg.data;
    console.log(msg);
    // Go sends only raw bytes.
    if ("[object ArrayBuffer]" == recv.toString()) {
      var bytes = new Uint8Array(recv);
      line = String.fromCharCode.apply(null, bytes);
    } else {
      line = recv;
    }
    line = line.trim();
    log(line);
  }
}

function startChat() {
  currentMode = MODE.CHAT;
  $chatlog.className = "active";
  log("------- chat enabled! -------");
}

// Get DOM elements and setup interactions.
function init() {
  console.log("loaded");
  // Setup chatwindow.
  $chatlog = document.getElementById('chatlog');
  $chatlog.value = "";

  $send = document.getElementById('send');
  $send.onclick = acceptInput

  $input = document.getElementById('input');
  $input.focus();
  $input.onkeydown = function(e) {
    if (13 == e.keyCode) {  // enter
      $send.onclick();
    }
  }
  welcome();
}

var log = function(msg) {
  $chatlog.value += msg + "\n";
  console.log(msg);
  // Scroll to latest.
  $chatlog.scrollTop = $chatlog.scrollHeight;
}

window.onload = init;

//
// some code sourced from flashproxy.js
// TODO: refactor / webrtc-afy it
//

/* Does the WebSocket implementation in this browser support binary frames? (RFC
   6455 section 5.6.) If not, we have to use base64-encoded text frames. It is
   assumed that the client and relay endpoints always support binary frames. */
function have_websocket_binary_frames() {
  var BROWSERS = [
    { idString: "Chrome", verString: "Chrome", version: 16 },
    { idString: "Safari", verString: "Version", version: 6 },
    { idString: "Firefox", verString: "Firefox", version: 11 }
  ];
  var ua;

  ua = window.navigator.userAgent;
  if (!ua)
    return false;

  for (var i = 0; i < BROWSERS.length; i++) {
    var matches, reg;

    reg = "\\b" + BROWSERS[i].idString + "\\b";
    if (!ua.match(new RegExp(reg, "i")))
        continue;
    reg = "\\b" + BROWSERS[i].verString + "\\/(\\d+)";
    matches = ua.match(new RegExp(reg, "i"));
    return matches !== null && Number(matches[1]) >= BROWSERS[i].version;
  }

  return false;
}

function make_websocket(addr) {
  var url;
  var ws;

  url = build_url("ws", addr.host, addr.port, "/");

  if (have_websocket_binary_frames())
    ws = new WebSocket(url);
  else
    ws = new WebSocket(url, "base64");
  /* "User agents can use this as a hint for how to handle incoming binary
     data: if the attribute is set to 'blob', it is safe to spool it to disk,
     and if it is set to 'arraybuffer', it is likely more efficient to keep
     the data in memory." */
  ws.binaryType = "arraybuffer";

  return ws;
}

function Snowflake() {
  if (HEADLESS) {
    /* No badge. */
  } else if (DEBUG) {
    this.badge_elem = debug_div;
  } else {
    this.badge = new Badge();
    this.badge_elem = this.badge.elem;
  }
  if (this.badge_elem)
    this.badge_elem.setAttribute("id", "flashproxy-badge");

  this.proxy_pairs = [];

  this.start = function() {
    var client_addr;
    var relay_addr;
    var rate_limit_bytes;
    log("Snowflake starting.")
    // TODO: Facilitator interaction
  };

  this.proxy_main = function() {
    var params;
    var base_url, url;
    var xhr;

    if (this.proxy_pairs.length >= this.max_num_clients * CONNECTIONS_PER_CLIENT) {
      setTimeout(this.proxy_main.bind(this), this.facilitator_poll_interval * 1000);
      return;
    }

    params = [["r", "1"]];
    params.push(["transport", "websocket"]);
    params.push(["transport", "webrtc"]);
  };

  this.begin_proxy = function(client_addr, relay_addr) {
    for (var i=0; i<CONNECTIONS_PER_CLIENT; i++) {
      this.make_proxy_pair(client_addr, relay_addr);
    }
  };

  this.make_proxy_pair = function(client_addr, relay_addr) {
    var proxy_pair;

    proxy_pair = new ProxyPair(client_addr, relay_addr, this.rate_limit);
    this.proxy_pairs.push(proxy_pair);
    proxy_pair.cleanup_callback = function(event) {
        /* Delete from the list of active proxy pairs. */
        this.proxy_pairs.splice(this.proxy_pairs.indexOf(proxy_pair), 1);
        if (this.badge)
            this.badge.proxy_end();
    }.bind(this);
    try {
        proxy_pair.connect();
    } catch (err) {
        puts("ProxyPair: exception while connecting: " + safe_repr(err.message) + ".");
        return;
    }

    if (this.badge)
        this.badge.proxy_begin();
  };

  /* Cease all network operations and prevent any future ones. */
  this.cease_operation = function() {
    this.start = function() { };
    this.proxy_main = function() { };
    this.make_proxy_pair = function(client_addr, relay_addr) { };
    while (this.proxy_pairs.length > 0)
      this.proxy_pairs.pop().close();
  };

  this.disable = function() {
    puts("Disabling.");
    this.cease_operation();
    if (this.badge)
      this.badge.disable();
  };

  this.die = function() {
    puts("Dying.");
    this.cease_operation();
    if (this.badge)
      this.badge.die();
  };
}

/* An instance of a client-relay connection. */
function ProxyPair(client_addr, relay_addr, rate_limit) {
  var MAX_BUFFER = 10 * 1024 * 1024;

  function log(s) {
    if (!SAFE_LOGGING) {
        s = format_addr(client_addr) + '|' + format_addr(relay_addr) + ' : ' + s
    }
    // puts(s)
    window.log(s)
  }

  this.client_addr = client_addr;
  this.relay_addr = relay_addr;
  this.rate_limit = rate_limit;

  this.c2r_schedule = [];
  this.r2c_schedule = [];

  this.running = true;
  this.flush_timeout_id = null;

  /* This callback function can be overridden by external callers. */
  this.cleanup_callback = function() {
  };

  this.connect = function() {
    log("Client: connecting.");
    this.client_s = make_websocket(this.client_addr);

    /* Try to connect to the client first (since that is more likely to
       fail) and only after that try to connect to the relay. */
    this.client_s.label = "Client";
    this.client_s.onopen = this.client_onopen_callback;
    this.client_s.onclose = this.onclose_callback;
    this.client_s.onerror = this.onerror_callback;
    this.client_s.onmessage = this.onmessage_client_to_relay;
  };

  this.client_onopen_callback = function(event) {
    var ws = event.target;

    log(ws.label + ": connected.");
    log("Relay: connecting.");
    this.relay_s = make_websocket(this.relay_addr);

    this.relay_s.label = "Relay";
    this.relay_s.onopen = this.relay_onopen_callback;
    this.relay_s.onclose = this.onclose_callback;
    this.relay_s.onerror = this.onerror_callback;
    this.relay_s.onmessage = this.onmessage_relay_to_client;
  }.bind(this);

  this.relay_onopen_callback = function(event) {
    var ws = event.target;

    log(ws.label + ": connected.");
  }.bind(this);

  this.maybe_cleanup = function() {
    if (this.running && is_closed(this.client_s) && is_closed(this.relay_s)) {
        this.running = false;
        this.cleanup_callback();
        return true;
    }
    return false;
  }

  this.onclose_callback = function(event) {
    var ws = event.target;

    log(ws.label + ": closed.");
    this.flush();

    if (this.maybe_cleanup()) {
        puts("Complete.");
    }
  }.bind(this);

  this.onerror_callback = function(event) {
    var ws = event.target;

    log(ws.label + ": error.");
    this.close();

    // we can't rely on onclose_callback to cleanup, since one common error
    // case is when the client fails to connect and the relay never starts.
    // in that case close() is a NOP and onclose_callback is never called.
    this.maybe_cleanup();
  }.bind(this);

  this.onmessage_client_to_relay = function(event) {
    this.c2r_schedule.push(event.data);
    this.flush();
  }.bind(this);

  this.onmessage_relay_to_client = function(event) {
    this.r2c_schedule.push(event.data);
    this.flush();
  }.bind(this);

  function is_open(ws) {
    return ws !== undefined && ws.readyState === WebSocket.OPEN;
  }

  function is_closed(ws) {
    return ws === undefined || ws.readyState === WebSocket.CLOSED;
  }

  this.close = function() {
    if (!is_closed(this.client_s))
        this.client_s.close();
    if (!is_closed(this.relay_s))
        this.relay_s.close();
  };

  /* Send as much data as the rate limit currently allows. */
  this.flush = function() {
    var busy;

    if (this.flush_timeout_id)
        clearTimeout(this.flush_timeout_id);
    this.flush_timeout_id = null;

    busy = true;
    while (busy && !this.rate_limit.is_limited()) {
      var chunk;
      busy = false;

      if (is_open(this.client_s) &&
          this.client_s.bufferedAmount < MAX_BUFFER &&
          this.r2c_schedule.length > 0) {
        chunk = this.r2c_schedule.shift();
        this.rate_limit.update(chunk.length);
        this.client_s.send(chunk);
        busy = true;
      }
      if (is_open(this.relay_s) &&
          this.relay_s.bufferedAmount < MAX_BUFFER &&
          this.c2r_schedule.length > 0) {
        chunk = this.c2r_schedule.shift();
        this.rate_limit.update(chunk.length);
        this.relay_s.send(chunk);
        busy = true;
      }
    }

    if (is_closed(this.relay_s) &&
        !is_closed(this.client_s) &&
        this.client_s.bufferedAmount === 0 &&
        this.r2c_schedule.length === 0) {
      log("Client: closing.");
      this.client_s.close();
    }
    if (is_closed(this.client_s) &&
        !is_closed(this.relay_s) &&
        this.relay_s.bufferedAmount === 0 &&
        this.c2r_schedule.length === 0) {
      log("Relay: closing.");
      this.relay_s.close();
    }

    if (this.r2c_schedule.length > 0 ||
        (is_open(this.client_s) && this.client_s.bufferedAmount > 0) ||
        this.c2r_schedule.length > 0 ||
        (is_open(this.relay_s) && this.relay_s.bufferedAmount > 0))
      this.flush_timeout_id = setTimeout(
        this.flush.bind(this), this.rate_limit.when() * 1000);
  };
}

