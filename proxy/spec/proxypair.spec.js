/* global expect, it, describe, spyOn */

/*
jasmine tests for Snowflake proxypair
*/

// Replacement for MessageEvent constructor.
// https://developer.mozilla.org/en-US/docs/Web/API/MessageEvent/MessageEvent
var MessageEvent = function(type, init) {
  return init;
};

// Asymmetic matcher that checks that two arrays have the same contents.
var arrayMatching = function(sample) {
  return {
    asymmetricMatch: function(other) {
      var _, a, b, i, j, len;
      a = new Uint8Array(sample);
      b = new Uint8Array(other);
      if (a.length !== b.length) {
        return false;
      }
      for (i = j = 0, len = a.length; j < len; i = ++j) {
        _ = a[i];
        if (a[i] !== b[i]) {
          return false;
        }
      }
      return true;
    },
    jasmineToString: function() {
      return '<arrayMatchine(' + jasmine.pp(sample) + ')>';
    }
  };
};

describe('ProxyPair', function() {

  var config, destination, fakeRelay, pp, rateLimit;
  fakeRelay = Parse.address('0.0.0.0:12345');
  rateLimit = new DummyRateLimit;
  config = new Config;
  destination = [];

  // Using the mock PeerConnection definition from spec/snowflake.spec.js
  var pp = new ProxyPair(fakeRelay, rateLimit, config.pcConfig);

  beforeEach(function() {
    return pp.begin();
  });

  it('begins webrtc connection', function() {
    return expect(pp.pc).not.toBeNull();
  });

  describe('accepts WebRTC offer from some client', function() {

    beforeEach(function() {
      return pp.begin();
    });

    it('rejects invalid offers', function() {
      expect(typeof pp.pc.setRemoteDescription).toBe("function");
      expect(pp.pc).not.toBeNull();
      expect(pp.receiveWebRTCOffer({})).toBe(false);
      expect(pp.receiveWebRTCOffer({
        type: 'answer'
      })).toBe(false);
    });

    it('accepts valid offers', function() {
      expect(pp.pc).not.toBeNull();
      expect(pp.receiveWebRTCOffer({
        type: 'offer',
        sdp: 'foo'
      })).toBe(true);
    });

  });

  it('responds with a WebRTC answer correctly', function() {
    spyOn(snowflake.broker, 'sendAnswer');
    pp.pc.onicecandidate({
      candidate: null
    });
    expect(snowflake.broker.sendAnswer).toHaveBeenCalled();
  });

  it('handles a new data channel correctly', function() {
    expect(pp.client).toBeNull();
    pp.pc.ondatachannel({
      channel: {}
    });
    expect(pp.client).not.toBeNull();
    expect(pp.client.onopen).not.toBeNull();
    expect(pp.client.onclose).not.toBeNull();
    expect(pp.client.onerror).not.toBeNull();
    expect(pp.client.onmessage).not.toBeNull();
  });

  it('connects to the relay once datachannel opens', function() {
    spyOn(pp, 'connectRelay');
    pp.active = true;
    pp.client.onopen();
    expect(pp.connectRelay).toHaveBeenCalled();
  });

  it('connects to a relay', function() {
    pp.connectRelay();
    expect(pp.relay.onopen).not.toBeNull();
    expect(pp.relay.onclose).not.toBeNull();
    expect(pp.relay.onerror).not.toBeNull();
    expect(pp.relay.onmessage).not.toBeNull();
  });

  describe('flushes data between client and relay', function() {

    it('proxies data from client to relay', function() {
      var msg;
      pp.pc.ondatachannel({
        channel: {
          bufferedAmount: 0,
          readyState: "open",
          send: function(data) {}
        }
      });
      spyOn(pp.client, 'send');
      spyOn(pp.relay, 'send');
      msg = new MessageEvent("message", {
        data: Uint8Array.from([1, 2, 3]).buffer
      });
      pp.onClientToRelayMessage(msg);
      pp.flush();
      expect(pp.client.send).not.toHaveBeenCalled();
      expect(pp.relay.send).toHaveBeenCalledWith(arrayMatching([1, 2, 3]));
    });

    it('proxies data from relay to client', function() {
      var msg;
      spyOn(pp.client, 'send');
      spyOn(pp.relay, 'send');
      msg = new MessageEvent("message", {
        data: Uint8Array.from([4, 5, 6]).buffer
      });
      pp.onRelayToClientMessage(msg);
      pp.flush();
      expect(pp.client.send).toHaveBeenCalledWith(arrayMatching([4, 5, 6]));
      expect(pp.relay.send).not.toHaveBeenCalled();
    });

    it('sends nothing with nothing to flush', function() {
      spyOn(pp.client, 'send');
      spyOn(pp.relay, 'send');
      pp.flush();
      expect(pp.client.send).not.toHaveBeenCalled();
      expect(pp.relay.send).not.toHaveBeenCalled();
    });

  });

});

// TODO: rate limit tests
