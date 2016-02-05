###
jasmine tests for Snowflake
###

# Stubs to fake browser functionality.
class PeerConnection
class WebSocket
  OPEN: 1
  CLOSED: 0
ui =
  log: ->
  setActive: ->
log = ->

describe 'BuildUrl', ->
  it 'should parse just protocol and host', ->
    expect(buildUrl('http', 'example.com')).toBe 'http://example.com'
  it 'should handle different ports', ->
    expect buildUrl 'http', 'example.com', 80
      .toBe 'http://example.com'
    expect buildUrl 'http', 'example.com', 81
      .toBe 'http://example.com:81'
    expect buildUrl 'http', 'example.com', 443
      .toBe 'http://example.com:443'
    expect buildUrl 'http', 'example.com', 444
      .toBe 'http://example.com:444'
  it 'should handle paths', ->
    expect buildUrl 'http', 'example.com', 80, '/'
      .toBe 'http://example.com/'
    expect buildUrl 'http', 'example.com', 80,'/test?k=%#v'
      .toBe 'http://example.com/test%3Fk%3D%25%23v'
    expect buildUrl 'http', 'example.com', 80, '/test'
      .toBe 'http://example.com/test'
  it 'should handle params', ->
    expect buildUrl 'http', 'example.com', 80, '/test', [['k', '%#v']]
      .toBe 'http://example.com/test?k=%25%23v'
    expect buildUrl 'http', 'example.com', 80, '/test', [['a', 'b'], ['c', 'd']]
      .toBe 'http://example.com/test?a=b&c=d'
  it 'should handle ips', ->
    expect buildUrl 'http', '1.2.3.4'
      .toBe 'http://1.2.3.4'
    expect buildUrl 'http', '1:2::3:4'
      .toBe 'http://[1:2::3:4]'
  it 'should handle bogus', ->
    expect buildUrl 'http', 'bog][us'
      .toBe 'http://bog%5D%5Bus'
    expect buildUrl 'http', 'bog:u]s'
      .toBe 'http://bog%3Au%5Ds'

describe 'Parse', ->

  describe 'cookie', ->
    it 'parses correctly', ->
      expect Parse.cookie ''
        .toEqual {}
      expect Parse.cookie 'a=b'
        .toEqual { a: 'b' }
      expect Parse.cookie 'a=b=c'
        .toEqual { a: 'b=c' }
      expect Parse.cookie 'a=b; c=d'
        .toEqual { a: 'b', c: 'd' }
      expect Parse.cookie 'a=b ; c=d'
        .toEqual { a: 'b', c: 'd' }
      expect Parse.cookie 'a= b'
        .toEqual { a: 'b' }
      expect Parse.cookie 'a='
        .toEqual { a: '' }
      expect Parse.cookie 'key'
        .toBeNull()
      expect Parse.cookie 'key=%26%20'
        .toEqual { key: '& ' }
      expect Parse.cookie 'a=\'\''
        .toEqual { a: '\'\'' }

  describe 'address', ->
    it 'parses IPv4', ->
      expect Parse.address ''
        .toBeNull()
      expect Parse.address '3.3.3.3:4444'
        .toEqual { host: '3.3.3.3', port: 4444 }
      expect Parse.address '3.3.3.3'
        .toBeNull()
      expect Parse.address '3.3.3.3:0x1111'
        .toBeNull()
      expect Parse.address '3.3.3.3:-4444'
        .toBeNull()
      expect Parse.address '3.3.3.3:65536'
        .toBeNull()
    it 'parses IPv6', ->
      expect Parse.address '[1:2::a:f]:4444'
        .toEqual { host: '1:2::a:f', port: 4444 }
      expect Parse.address '[1:2::a:f]'
        .toBeNull()
      expect Parse.address '[1:2::a:f]:0x1111'
        .toBeNull()
      expect Parse.address '[1:2::a:f]:-4444'
        .toBeNull()
      expect Parse.address '[1:2::a:f]:65536'
        .toBeNull()
      expect Parse.address '[1:2::ffff:1.2.3.4]:4444'
        .toEqual { host: '1:2::ffff:1.2.3.4', port: 4444 }

describe 'query string', ->
  it 'should parse correctly', ->
    expect Query.parse ''
      .toEqual {}
    expect Query.parse 'a=b'
      .toEqual { a: 'b' }
    expect Query.parse 'a=b=c'
      .toEqual { a: 'b=c' }
    expect Query.parse 'a=b&c=d'
      .toEqual { a: 'b', c: 'd' }
    expect Query.parse 'client=&relay=1.2.3.4%3A9001'
      .toEqual { client: '', relay: '1.2.3.4:9001' }
    expect Query.parse 'a=b%26c=d'
      .toEqual { a: 'b&c=d' }
    expect Query.parse 'a%3db=d'
      .toEqual { 'a=b': 'd' }
    expect Query.parse 'a=b+c%20d'
      .toEqual { 'a': 'b c d' }
    expect Query.parse 'a=b+c%2bd'
      .toEqual { 'a': 'b c+d' }
    expect Query.parse 'a+b=c'
      .toEqual { 'a b': 'c' }
    expect Query.parse 'a=b+c+d'
      .toEqual { a: 'b c d' }
  it 'uses the first appearance of duplicate key', ->
    expect Query.parse 'a=b&c=d&a=e'
      .toEqual { a: 'b', c: 'd' }
    expect Query.parse 'a'
      .toEqual { a: '' }
    expect Query.parse '=b'
      .toEqual { '': 'b' }
    expect Query.parse '&a=b'
      .toEqual { '': '', a: 'b' }
    expect Query.parse 'a=b&'
      .toEqual { a: 'b', '':'' }
    expect Query.parse 'a=b&&c=d'
      .toEqual { a: 'b', '':'', c: 'd' }

describe 'Params', ->
  describe 'bool', ->
    getBool = (query) ->
      Params.getBool (Query.parse query), 'param', false
    it 'parses correctly', ->
      expect(getBool 'param=true').toBe true
      expect(getBool 'param').toBe true
      expect(getBool 'param=').toBe true
      expect(getBool 'param=1').toBe true
      expect(getBool 'param=0').toBe false
      expect(getBool 'param=false').toBe false
      expect(getBool 'param=unexpected').toBeNull()
      expect(getBool 'pram=true').toBe false

  describe 'address', ->
    DEFAULT = { host: '1.1.1.1', port: 2222 }
    getAddress = (query) ->
      Params.getAddress query, 'addr', DEFAULT
    it 'parses correctly', ->
      expect(getAddress {}).toEqual DEFAULT
      expect getAddress { addr: '3.3.3.3:4444' }
        .toEqual { host: '3.3.3.3', port: 4444 }
      expect getAddress { x: '3.3.3.3:4444' }
        .toEqual DEFAULT
      expect getAddress { addr: '---' }
        .toBeNull()

describe 'ProxyPair', ->
  fakeRelay = Parse.address '0.0.0.0:12345'
  rateLimit = new DummyRateLimit()
  destination = []
  fakeClient = send: (d) -> destination.push d
  # Fake snowflake to interact with
  snowflake = {
    broker:
      sendAnswer: ->
  }
  pp = new ProxyPair(fakeClient, fakeRelay, rateLimit)

  it 'begins webrtc connection', ->
    pp.begin()
    expect(pp.pc).not.toBeNull()

  it 'accepts WebRTC offer from some client', ->
    it 'rejects invalid offers', ->
      expect(pp.receiveWebRTCOffer {}).toBe false
      expect pp.receiveWebRTCOffer {
        type: 'answer'
      }.toBeFalse()
    it 'accepts valid offers', ->
      goodOffer = {
        type: 'offer'
        sdp: 'foo'
      }
      expect(pp.receiveWebRTCOffer goodOffer).toBe true

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

  it 'flushes data between client and relay', ->

    it 'proxies data from client to relay', ->
      spyOn pp.relay, 'send'
      pp.c2rSchedule.push { data: 'foo' }
      pp.flush()
      expect(pp.client.send).not.toHaveBeenCalled()
      expect(pp.relay.send).toHaveBeenCalledWith 'foo'

    it 'proxies data from relay to client', ->
      spyOn pp.client, 'send'
      pp.r2cSchedule.push { data: 'bar' }
      pp.flush()
      expect(pp.client.send).toHaveBeenCalledWith 'bar'
      expect(pp.relay.send).not.toHaveBeenCalled()

    it 'sends nothing with nothing to flush', ->
      pp.flush()
      expect(pp.client.send).not.toHaveBeenCalled()
      expect(pp.relay.send).not.toHaveBeenCalled()

  # TODO: rate limit tests
