###
jasmine tests for Snowflake
###

# Fake browser functionality:
class PeerConnection
  setRemoteDescription: ->
    true
  send: (data) ->
class SessionDescription
  type: 'offer'
class WebSocket
  OPEN: 1
  CLOSED: 0
  constructor: ->
    @bufferedAmount = 0
  send: (data) ->

log = ->

config = new Config
ui = new UI

class FakeBroker
  getClientOffer: -> new Promise((F,R) -> {})

describe 'Snowflake', ->

  it 'constructs correctly', ->
    s = new Snowflake(config, ui, { fake: 'broker' })
    expect(s.rateLimit).not.toBeNull()
    expect(s.broker).toEqual { fake: 'broker' }
    expect(s.ui).not.toBeNull()
    expect(s.retries).toBe 0

  it 'sets relay address correctly', ->
    s = new Snowflake(config, ui, null)
    s.setRelayAddr 'foo'
    expect(s.relayAddr).toEqual 'foo'

  it 'initalizes WebRTC connection', ->
    s = new Snowflake(config, ui, new FakeBroker())
    spyOn(s.broker, 'getClientOffer').and.callThrough()
    s.beginWebRTC()
    expect(s.retries).toBe 1
    expect(s.broker.getClientOffer).toHaveBeenCalled()

  it 'receives SDP offer and sends answer', ->
    s = new Snowflake(config, ui, new FakeBroker())
    pair = { receiveWebRTCOffer: -> }
    spyOn(pair, 'receiveWebRTCOffer').and.returnValue true
    spyOn(s, 'sendAnswer')
    s.receiveOffer pair, '{"type":"offer","sdp":"foo"}'
    expect(s.sendAnswer).toHaveBeenCalled()

  it 'does not send answer when receiving invalid offer', ->
    s = new Snowflake(config, ui, new FakeBroker())
    pair = { receiveWebRTCOffer: -> }
    spyOn(pair, 'receiveWebRTCOffer').and.returnValue false
    spyOn(s, 'sendAnswer')
    s.receiveOffer pair, '{"type":"not a good offer","sdp":"foo"}'
    expect(s.sendAnswer).not.toHaveBeenCalled()

  it 'can make a proxypair', ->
    s = new Snowflake(config, ui, new FakeBroker())
    s.makeProxyPair()
    expect(s.proxyPairs.length).toBe 1
