/* global expect, it, describe, spyOn, Snowflake, Config, UI */

/*
jasmine tests for Snowflake
*/

// Fake browser functionality:
class PeerConnection {
  setRemoteDescription() {
    return true;
  }
  send() {}
}

class SessionDescription {}
SessionDescription.prototype.type = 'offer';

class WebSocket {
  constructor() {
    this.bufferedAmount = 0;
  }
  send() {}
}
WebSocket.prototype.OPEN = 1;
WebSocket.prototype.CLOSED = 0;

var log = function() {};

var config = new Config();

var ui = new UI();

class FakeBroker {
  getClientOffer() {
    return new Promise(function() {
      return {};
    });
  }
}

describe('Snowflake', function() {

  it('constructs correctly', function() {
    var s;
    s = new Snowflake(config, ui, {
      fake: 'broker'
    });
    expect(s.rateLimit).not.toBeNull();
    expect(s.broker).toEqual({
      fake: 'broker'
    });
    expect(s.ui).not.toBeNull();
    expect(s.retries).toBe(0);
  });

  it('sets relay address correctly', function() {
    var s;
    s = new Snowflake(config, ui, null);
    s.setRelayAddr('foo');
    expect(s.relayAddr).toEqual('foo');
  });

  it('initalizes WebRTC connection', function() {
    var s;
    s = new Snowflake(config, ui, new FakeBroker());
    spyOn(s.broker, 'getClientOffer').and.callThrough();
    s.beginWebRTC();
    expect(s.retries).toBe(1);
    expect(s.broker.getClientOffer).toHaveBeenCalled();
  });

  it('receives SDP offer and sends answer', function() {
    var pair, s;
    s = new Snowflake(config, ui, new FakeBroker());
    pair = {
      receiveWebRTCOffer: function() {}
    };
    spyOn(pair, 'receiveWebRTCOffer').and.returnValue(true);
    spyOn(s, 'sendAnswer');
    s.receiveOffer(pair, '{"type":"offer","sdp":"foo"}');
    expect(s.sendAnswer).toHaveBeenCalled();
  });

  it('does not send answer when receiving invalid offer', function() {
    var pair, s;
    s = new Snowflake(config, ui, new FakeBroker());
    pair = {
      receiveWebRTCOffer: function() {}
    };
    spyOn(pair, 'receiveWebRTCOffer').and.returnValue(false);
    spyOn(s, 'sendAnswer');
    s.receiveOffer(pair, '{"type":"not a good offer","sdp":"foo"}');
    expect(s.sendAnswer).not.toHaveBeenCalled();
  });

  it('can make a proxypair', function() {
    var s;
    s = new Snowflake(config, ui, new FakeBroker());
    s.makeProxyPair();
    expect(s.proxyPairs.length).toBe(1);
  });

});
