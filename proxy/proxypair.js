/* global snowflake, log, dbg, Util, PeerConnection, Parse, WS */

/*
Represents a single:

   client <-- webrtc --> snowflake <-- websocket --> relay

Every ProxyPair has a Snowflake ID, which is necessary when responding to the
Broker with an WebRTC answer.
*/

class ProxyPair {

  /*
  Constructs a ProxyPair where:
  - @relayAddr is the destination relay
  - @rateLimit specifies a rate limit on traffic
  */
  constructor(relayAddr, rateLimit, pcConfig) {
    this.prepareDataChannel = this.prepareDataChannel.bind(this);
    this.connectRelay = this.connectRelay.bind(this);
    this.onClientToRelayMessage = this.onClientToRelayMessage.bind(this);
    this.onRelayToClientMessage = this.onRelayToClientMessage.bind(this);
    this.onError = this.onError.bind(this);
    this.flush = this.flush.bind(this);

    this.relayAddr = relayAddr;
    this.rateLimit = rateLimit;
    this.pcConfig = pcConfig;
    this.id = Util.genSnowflakeID();
    this.c2rSchedule = [];
    this.r2cSchedule = [];
  }

  // Prepare a WebRTC PeerConnection and await for an SDP offer.
  begin() {
    this.pc = new PeerConnection(this.pcConfig, {
      optional: [
        {
          DtlsSrtpKeyAgreement: true
        },
        {
          RtpDataChannels: false
        }
      ]
    });
    this.pc.onicecandidate = (evt) => {
      // Browser sends a null candidate once the ICE gathering completes.
      if (null === evt.candidate) {
        // TODO: Use a promise.all to tell Snowflake about all offers at once,
        // once multiple proxypairs are supported.
        dbg('Finished gathering ICE candidates.');
        return snowflake.broker.sendAnswer(this.id, this.pc.localDescription);
      }
    };
    // OnDataChannel triggered remotely from the client when connection succeeds.
    return this.pc.ondatachannel = (dc) => {
      var channel;
      channel = dc.channel;
      dbg('Data Channel established...');
      this.prepareDataChannel(channel);
      return this.client = channel;
    };
  }

  receiveWebRTCOffer(offer) {
    if ('offer' !== offer.type) {
      log('Invalid SDP received -- was not an offer.');
      return false;
    }
    try {
      this.pc.setRemoteDescription(offer);
    } catch (error) {
      log('Invalid SDP message.');
      return false;
    }
    dbg('SDP ' + offer.type + ' successfully received.');
    return true;
  }

  // Given a WebRTC DataChannel, prepare callbacks.
  prepareDataChannel(channel) {
    channel.onopen = () => {
      log('WebRTC DataChannel opened!');
      snowflake.ui.setActive(true);
      // This is the point when the WebRTC datachannel is done, so the next step
      // is to establish websocket to the server.
      return this.connectRelay();
    };
    channel.onclose = () => {
      log('WebRTC DataChannel closed.');
      snowflake.ui.setStatus('disconnected by webrtc.');
      snowflake.ui.setActive(false);
      this.flush();
      return this.close();
    };
    channel.onerror = function() {
      return log('Data channel error!');
    };
    channel.binaryType = "arraybuffer";
    return channel.onmessage = this.onClientToRelayMessage;
  }

  // Assumes WebRTC datachannel is connected.
  connectRelay() {
    var params, peer_ip, ref;
    dbg('Connecting to relay...');
    // Get a remote IP address from the PeerConnection, if possible. Add it to
    // the WebSocket URL's query string if available.
    // MDN marks remoteDescription as "experimental". However the other two
    // options, currentRemoteDescription and pendingRemoteDescription, which
    // are not marked experimental, were undefined when I tried them in Firefox
    // 52.2.0.
    // https://developer.mozilla.org/en-US/docs/Web/API/RTCPeerConnection/remoteDescription
    peer_ip = Parse.ipFromSDP((ref = this.pc.remoteDescription) != null ? ref.sdp : void 0);
    params = [];
    if (peer_ip != null) {
      params.push(["client_ip", peer_ip]);
    }
    var relay = this.relay = WS.makeWebsocket(this.relayAddr, params);
    this.relay.label = 'websocket-relay';
    this.relay.onopen = () => {
      if (this.timer) {
        clearTimeout(this.timer);
        this.timer = 0;
      }
      log(relay.label + ' connected!');
      return snowflake.ui.setStatus('connected');
    };
    this.relay.onclose = () => {
      log(relay.label + ' closed.');
      snowflake.ui.setStatus('disconnected.');
      snowflake.ui.setActive(false);
      this.flush();
      return this.close();
    };
    this.relay.onerror = this.onError;
    this.relay.onmessage = this.onRelayToClientMessage;
    // TODO: Better websocket timeout handling.
    return this.timer = setTimeout((() => {
      if (0 === this.timer) {
        return;
      }
      log(relay.label + ' timed out connecting.');
      return relay.onclose();
    }), 5000);
  }

  // WebRTC --> websocket
  onClientToRelayMessage(msg) {
    dbg('WebRTC --> websocket data: ' + msg.data.byteLength + ' bytes');
    this.c2rSchedule.push(msg.data);
    return this.flush();
  }

  // websocket --> WebRTC
  onRelayToClientMessage(event) {
    dbg('websocket --> WebRTC data: ' + event.data.byteLength + ' bytes');
    this.r2cSchedule.push(event.data);
    return this.flush();
  }

  onError(event) {
    var ws;
    ws = event.target;
    log(ws.label + ' error.');
    return this.close();
  }

  // Close both WebRTC and websocket.
  close() {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = 0;
    }
    if (this.webrtcIsReady()) {
      this.client.close();
    }
    if (this.peerConnOpen()) {
      this.pc.close();
    }
    if (this.relayIsReady()) {
      this.relay.close();
    }
    this.onCleanup();
  }

  // Send as much data in both directions as the rate limit currently allows.
  flush() {
    var busy, checkChunks;
    if (this.flush_timeout_id) {
      clearTimeout(this.flush_timeout_id);
    }
    this.flush_timeout_id = null;
    busy = true;
    checkChunks = () => {
      var chunk;
      busy = false;
      // WebRTC --> websocket
      if (this.relayIsReady() && this.relay.bufferedAmount < this.MAX_BUFFER && this.c2rSchedule.length > 0) {
        chunk = this.c2rSchedule.shift();
        this.rateLimit.update(chunk.byteLength);
        this.relay.send(chunk);
        busy = true;
      }
      // websocket --> WebRTC
      if (this.webrtcIsReady() && this.client.bufferedAmount < this.MAX_BUFFER && this.r2cSchedule.length > 0) {
        chunk = this.r2cSchedule.shift();
        this.rateLimit.update(chunk.byteLength);
        this.client.send(chunk);
        return busy = true;
      }
    };
    while (busy && !this.rateLimit.isLimited()) {
      checkChunks();
    }
    if (this.r2cSchedule.length > 0 || this.c2rSchedule.length > 0 || (this.relayIsReady() && this.relay.bufferedAmount > 0) || (this.webrtcIsReady() && this.client.bufferedAmount > 0)) {
      return this.flush_timeout_id = setTimeout(this.flush, this.rateLimit.when() * 1000);
    }
  }

  webrtcIsReady() {
    return null !== this.client && 'open' === this.client.readyState;
  }

  relayIsReady() {
    return (null !== this.relay) && (WebSocket.OPEN === this.relay.readyState);
  }

  isClosed(ws) {
    return void 0 === ws || WebSocket.CLOSED === ws.readyState;
  }

  peerConnOpen() {
    return (null !== this.pc) && ('closed' !== this.pc.connectionState);
  }

}

ProxyPair.prototype.MAX_BUFFER = 10 * 1024 * 1024;

ProxyPair.prototype.pc = null;
ProxyPair.prototype.client = null; // WebRTC Data channel
ProxyPair.prototype.relay = null; // websocket

ProxyPair.prototype.timer = 0;
ProxyPair.prototype.flush_timeout_id = null;

ProxyPair.prototype.onCleanup = null;
