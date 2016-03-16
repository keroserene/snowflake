###
jasmine tests for Snowflake
###

query = {}
# Fake browser functionality:
class PeerConnection
class RTCSessionDescription
  type: 'offer'
class WebSocket
  OPEN: 1
  CLOSED: 0
log = ->
class FakeUI
  log: ->
  setActive: ->
  setStatus: ->
fakeUI = new FakeUI()
class FakeBroker
  getClientOffer: -> new Promise((F,R) -> {})
# Fake snowflake to interact with
snowflake =
  ui: fakeUI
  broker:
    sendAnswer: ->
  state: MODE.INIT

describe 'Snowflake', ->

  it 'constructs correctly', ->
    s = new Snowflake({ fake: 'broker' }, fakeUI)
    query['ratelimit'] = 'off'
    expect(s.rateLimit).not.toBeNull()
    expect(s.broker).toEqual { fake: 'broker' }
    expect(s.ui).not.toBeNull()
    expect(s.retries).toBe 0

  it 'sets relay address correctly', ->
    s = new Snowflake(null, fakeUI)
    s.setRelayAddr 'foo'
    expect(s.relayAddr).toEqual 'foo'

  it 'initalizes WebRTC connection', ->
    s = new Snowflake(new FakeBroker(), fakeUI)
    spyOn(s.broker, 'getClientOffer').and.callThrough()
    s.beginWebRTC()
    expect(s.retries).toBe 1
    expect(s.broker.getClientOffer).toHaveBeenCalled()

  it 'receives SDP offer and sends answer', ->
    s = new Snowflake(new FakeBroker(), fakeUI)
    pair = { receiveWebRTCOffer: -> }
    spyOn(pair, 'receiveWebRTCOffer').and.returnValue true
    spyOn(s, 'sendAnswer')
    s.receiveOffer pair, '{"type":"offer","sdp":"foo"}'
    expect(s.sendAnswer).toHaveBeenCalled()

  it 'does not send answer when receiving invalid offer', ->
    s = new Snowflake(new FakeBroker(), fakeUI)
    pair = { receiveWebRTCOffer: -> }
    spyOn(pair, 'receiveWebRTCOffer').and.returnValue false
    spyOn(s, 'sendAnswer')
    s.receiveOffer pair, '{"type":"not a good offer","sdp":"foo"}'
    expect(s.sendAnswer).not.toHaveBeenCalled()

  it 'can make a proxypair', ->
    s = new Snowflake(new FakeBroker(), fakeUI)
    s.makeProxyPair()
    expect(s.proxyPairs.length).toBe 2

  it 'gives a dialog when closing, only while active', ->
    silenceNotifications = false
    snowflake.state = MODE.WEBRTC_READY
    msg = window.onbeforeunload()
    expect(snowflake.state).toBe MODE.WEBRTC_READY
    expect(msg).toBe CONFIRMATION_MESSAGE

    snowflake.state = MODE.INIT
    msg = window.onbeforeunload()
    expect(snowflake.state).toBe MODE.INIT
    expect(msg).toBe null

  it 'does not give a dialog when silent flag is on', ->
    silenceNotifications = true
    snowflake.state = MODE.WEBRTC_READY
    msg = window.onbeforeunload()
    expect(snowflake.state).toBe MODE.WEBRTC_READY
    expect(msg).toBe null
