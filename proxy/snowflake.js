/* global log, dbg, DummyRateLimit, BucketRateLimit, SessionDescription, ProxyPair */

/*
A JavaScript WebRTC snowflake proxy

Uses WebRTC from the client, and Websocket to the server.

Assume that the webrtc client plugin is always the offerer, in which case
this proxy must always act as the answerer.

TODO: More documentation
*/

// Minimum viable snowflake for now - just 1 client.
class Snowflake {

  // Prepare the Snowflake with a Broker (to find clients) and optional UI.
  constructor(config, ui, broker) {
    // Receive an SDP offer from some client assigned by the Broker,
    // |pair| - an available ProxyPair.
    this.receiveOffer = this.receiveOffer.bind(this);
    this.config = config;
    this.ui = ui;
    this.broker = broker;
    this.proxyPairs = [];
    if (void 0 === this.config.rateLimitBytes) {
      this.rateLimit = new DummyRateLimit();
    } else {
      this.rateLimit = new BucketRateLimit(this.config.rateLimitBytes * this.config.rateLimitHistory, this.config.rateLimitHistory);
    }
    this.retries = 0;
  }

  // Set the target relay address spec, which is expected to be websocket.
  // TODO: Should potentially fetch the target from broker later, or modify
  // entirely for the Tor-independent version.
  setRelayAddr(relayAddr) {
    this.relayAddr = relayAddr;
    log('Using ' + relayAddr.host + ':' + relayAddr.port + ' as Relay.');
    return true;
  }

  // Initialize WebRTC PeerConnection, which requires beginning the signalling
  // process. |pollBroker| automatically arranges signalling.
  beginWebRTC() {
    log('ProxyPair Slots: ' + this.proxyPairs.length);
    log('Snowflake IDs: ' + (this.proxyPairs.map(function(p) {
      return p.id;
    })).join(' | '));
    this.pollBroker();
    return this.pollInterval = setInterval((() => {
      return this.pollBroker();
    }), this.config.defaultBrokerPollInterval);
  }

  // Regularly poll Broker for clients to serve until this snowflake is
  // serving at capacity, at which point stop polling.
  pollBroker() {
    var msg, pair, recv;
    // Poll broker for clients.
    pair = this.nextAvailableProxyPair();
    if (!pair) {
      log('At client capacity.');
      return;
    }
    // Do nothing until a new proxyPair is available.
    pair.active = true;
    msg = 'Polling for client ... ';
    if (this.retries > 0) {
      msg += '[retries: ' + this.retries + ']';
    }
    this.ui.setStatus(msg);
    recv = this.broker.getClientOffer(pair.id);
    recv.then((desc) => {
      if (pair.active) {
        if (!this.receiveOffer(pair, desc)) {
          return pair.active = false;
        }
        //set a timeout for channel creation
        return setTimeout((() => {
          if (!pair.running) {
            log('proxypair datachannel timed out waiting for open');
            pair.close();
            return pair.active = false;
          }
        }), 20000); // 20 second timeout
      }
    }, function() {
      return pair.active = false;
    });
    return this.retries++;
  }

  // Returns the first ProxyPair that's available to connect.
  nextAvailableProxyPair() {
    if (this.proxyPairs.length < this.config.connectionsPerClient) {
      return this.makeProxyPair(this.relayAddr);
    }
    return this.proxyPairs.find(function(pp) {
      return !pp.active;
    });
  }

  receiveOffer(pair, desc) {
    var e, offer, sdp;
    try {
      offer = JSON.parse(desc);
      dbg('Received:\n\n' + offer.sdp + '\n');
      sdp = new SessionDescription(offer);
      if (pair.receiveWebRTCOffer(sdp)) {
        this.sendAnswer(pair);
        return true;
      } else {
        return false;
      }
    } catch (error) {
      e = error;
      log('ERROR: Unable to receive Offer: ' + e);
      return false;
    }
  }

  sendAnswer(pair) {
    var fail, next;
    next = function(sdp) {
      dbg('webrtc: Answer ready.');
      return pair.pc.setLocalDescription(sdp).catch(fail);
    };
    fail = function() {
      pair.active = false
      return dbg('webrtc: Failed to create or set Answer');
    };
    return pair.pc.createAnswer().then(next).catch(fail);
  }

  makeProxyPair(relay) {
    var pair;
    pair = new ProxyPair(relay, this.rateLimit, this.config.pcConfig);
    this.proxyPairs.push(pair);
    pair.onCleanup = () => {
      var ind;
      // Delete from the list of active proxy pairs.
      ind = this.proxyPairs.indexOf(pair);
      if (ind > -1) {
        return this.proxyPairs.splice(ind, 1);
      }
    };
    pair.begin();
    return pair;
  }

  // Stop all proxypairs.
  disable() {
    var results;
    log('Disabling Snowflake.');
    clearInterval(this.pollInterval);
    results = [];
    while (this.proxyPairs.length > 0) {
      results.push(this.proxyPairs.pop().close());
    }
    return results;
  }

}

Snowflake.prototype.relayAddr = null;

Snowflake.prototype.rateLimit = null;

Snowflake.prototype.pollInterval = null;

Snowflake.prototype.retries = 0;

Snowflake.MESSAGE = {
  CONFIRMATION: 'You\'re currently serving a Tor user via Snowflake.'
};
