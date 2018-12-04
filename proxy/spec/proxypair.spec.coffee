###
jasmine tests for Snowflake proxypair
###

# Replacement for MessageEvent constructor.
# https://developer.mozilla.org/en-US/docs/Web/API/MessageEvent/MessageEvent
MessageEvent = (type, init) ->
  init

# Asymmetic matcher that checks that two arrays have the same contents.
arrayMatching = (sample) -> {
  asymmetricMatch: (other) ->
    a = new Uint8Array(sample)
    b = new Uint8Array(other)
    if a.length != b.length
      return false
    for _, i in a
      if a[i] != b[i]
        return false
    true
  jasmineToString: ->
    '<arrayMatchine(' + jasmine.pp(sample) + ')>'
}

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
      msg = new MessageEvent("message", { data: Uint8Array.from([1, 2, 3]).buffer })
      pp.onClientToRelayMessage(msg)
      pp.flush()
      expect(pp.client.send).not.toHaveBeenCalled()
      expect(pp.relay.send).toHaveBeenCalledWith arrayMatching([1, 2, 3])

    it 'proxies data from relay to client', ->
      spyOn pp.client, 'send'
      spyOn pp.relay, 'send'
      msg = new MessageEvent("message", { data: Uint8Array.from([4, 5, 6]).buffer })
      pp.onRelayToClientMessage(msg)
      pp.flush()
      expect(pp.client.send).toHaveBeenCalledWith arrayMatching([4, 5, 6])
      expect(pp.relay.send).not.toHaveBeenCalled()

    it 'sends nothing with nothing to flush', ->
      spyOn pp.client, 'send'
      spyOn pp.relay, 'send'
      pp.flush()
      expect(pp.client.send).not.toHaveBeenCalled()
      expect(pp.relay.send).not.toHaveBeenCalled()

# TODO: rate limit tests
