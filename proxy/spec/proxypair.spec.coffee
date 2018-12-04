###
jasmine tests for Snowflake proxypair
###


describe 'ProxyPair', ->
  fakeRelay = Parse.address '0.0.0.0:12345'
  rateLimit = new DummyRateLimit()
  destination = []
  # Using the mock PeerConnection definition from spec/snowflake.spec.coffee.
  pp = new ProxyPair(fakeRelay, rateLimit)

  beforeEach ->
    pp.begin()

  it 'begins webrtc connection', ->
    expect(pp.pc).not.toBeNull()

  describe 'accepts WebRTC offer from some client', ->
    beforeEach ->
      pp.begin()

    it 'rejects invalid offers', ->
      expect(typeof(pp.pc.setRemoteDescription)).toBe("function")
      expect(pp.pc).not.toBeNull()
      expect(pp.receiveWebRTCOffer {}).toBe false
      expect(pp.receiveWebRTCOffer {
        type: 'answer'
      }).toBe false
    it 'accepts valid offers', ->
      expect(pp.pc).not.toBeNull()
      expect(pp.receiveWebRTCOffer {
        type: 'offer'
        sdp: 'foo'
      }).toBe true

  it 'responds with a WebRTC answer correctly', ->
    spyOn snowflake.broker, 'sendAnswer'
    pp.pc.onicecandidate {
      candidate: null
    }
    expect(snowflake.broker.sendAnswer).toHaveBeenCalled()

  it 'handles a new data channel correctly', ->
    expect(pp.client).toBeNull()
    pp.pc.ondatachannel {
      channel: {}
    }
    expect(pp.client).not.toBeNull()
    expect(pp.client.onopen).not.toBeNull()
    expect(pp.client.onclose).not.toBeNull()
    expect(pp.client.onerror).not.toBeNull()
    expect(pp.client.onmessage).not.toBeNull()

  it 'connects to the relay once datachannel opens', ->
    spyOn pp, 'connectRelay'
    pp.client.onopen()
    expect(pp.connectRelay).toHaveBeenCalled()

  it 'connects to a relay', ->
    pp.connectRelay()
    expect(pp.relay.onopen).not.toBeNull()
    expect(pp.relay.onclose).not.toBeNull()
    expect(pp.relay.onerror).not.toBeNull()
    expect(pp.relay.onmessage).not.toBeNull()

  describe 'flushes data between client and relay', ->

    it 'proxies data from client to relay', ->
      pp.pc.ondatachannel {
        channel: {
          bufferedAmount: 0
          readyState: "open"
          send: (data) ->
        }
      }
      spyOn pp.client, 'send'
      spyOn pp.relay, 'send'
      pp.c2rSchedule.push 'foo'
      pp.flush()
      expect(pp.client.send).not.toHaveBeenCalled()
      expect(pp.relay.send).toHaveBeenCalledWith 'foo'

    it 'proxies data from relay to client', ->
      spyOn pp.client, 'send'
      spyOn pp.relay, 'send'
      pp.r2cSchedule.push 'bar'
      pp.flush()
      expect(pp.client.send).toHaveBeenCalledWith 'bar'
      expect(pp.relay.send).not.toHaveBeenCalled()

    it 'sends nothing with nothing to flush', ->
      spyOn pp.client, 'send'
      spyOn pp.relay, 'send'
      pp.flush()
      expect(pp.client.send).not.toHaveBeenCalled()
      expect(pp.relay.send).not.toHaveBeenCalled()

# TODO: rate limit tests
