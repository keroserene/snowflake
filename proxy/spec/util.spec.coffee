###
jasmine tests for Snowflake utils
###

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

  describe 'ipFromSDP', ->
    testCases = [
      # https://tools.ietf.org/html/rfc4566#section-5
      sdp: """
           v=0
           o=jdoe 2890844526 2890842807 IN IP4 10.47.16.5
           s=SDP Seminar
           i=A Seminar on the session description protocol
           u=http://www.example.com/seminars/sdp.pdf
           e=j.doe@example.com (Jane Doe)
           c=IN IP4 224.2.17.12/127
           t=2873397496 2873404696
           a=recvonly
           m=audio 49170 RTP/AVP 0
           m=video 51372 RTP/AVP 99
           a=rtpmap:99 h263-1998/90000
           """
      expected: '224.2.17.12'
    ,
      # Missing c= line
      sdp: """
           v=0
           o=jdoe 2890844526 2890842807 IN IP4 10.47.16.5
           s=SDP Seminar
           i=A Seminar on the session description protocol
           u=http://www.example.com/seminars/sdp.pdf
           e=j.doe@example.com (Jane Doe)
           t=2873397496 2873404696
           a=recvonly
           m=audio 49170 RTP/AVP 0
           m=video 51372 RTP/AVP 99
           a=rtpmap:99 h263-1998/90000
           """
      expected: undefined
    ,
      # Single line, IP address only
      sdp: "c=IN IP4 224.2.1.1\n"
      expected: '224.2.1.1'
    ,
      # Same, with TTL
      sdp: "c=IN IP4 224.2.1.1/127\n"
      expected: '224.2.1.1'
    ,
      # Same, with TTL and multicast addresses
      sdp: "c=IN IP4 224.2.1.1/127/3\n"
      expected: '224.2.1.1'
    ,
      # IPv6, address only
      sdp: "c=IN IP6 FF15::101\n"
      expected: 'ff15::101'
    ,
      # Same, with multicast addresses
      sdp: "c=IN IP6 FF15::101/3\n"
      expected: 'ff15::101'
    ,
      # Multiple c= lines
      sdp: """
           c=IN IP4 1.2.3.4
           c=IN IP4 5.6.7.8
           """
      expected: '1.2.3.4'
    ,
      # Modified from SDP sent by snowflake-client.
      sdp: """
           v=0
           o=- 7860378660295630295 2 IN IP4 127.0.0.1
           s=-
           t=0 0
           a=group:BUNDLE data
           a=msid-semantic: WMS
           m=application 54653 DTLS/SCTP 5000
           c=IN IP4 1.2.3.4
           a=candidate:3581707038 1 udp 2122260223 192.168.0.1 54653 typ host generation 0 network-id 1 network-cost 50
           a=candidate:2617212910 1 tcp 1518280447 192.168.0.1 59673 typ host tcptype passive generation 0 network-id 1 network-cost 50
           a=candidate:2082671819 1 udp 1686052607 1.2.3.4 54653 typ srflx raddr 192.168.0.1 rport 54653 generation 0 network-id 1 network-cost 50
           a=ice-ufrag:IBdf
           a=ice-pwd:G3lTrrC9gmhQx481AowtkhYz
           a=fingerprint:sha-256 53:F8:84:D9:3C:1F:A0:44:AA:D6:3C:65:80:D3:CB:6F:23:90:17:41:06:F9:9C:10:D8:48:4A:A8:B6:FA:14:A1
           a=setup:actpass
           a=mid:data
           a=sctpmap:5000 webrtc-datachannel 1024
           """
      expected: '1.2.3.4'
    ,
      # Improper character within IPv4
      sdp: """
           c=IN IP4 224.2z.1.1
           """
      expected: undefined
    ,
      # Improper character within IPv6
      sdp: """
           c=IN IP6 ff15:g::101
           """
      expected: undefined
    ,
      # Bogus "IP7" addrtype
      sdp: "c=IN IP7 1.2.3.4\n"
      expected: undefined
    ]

    it 'parses SDP', ->
      for test in testCases
        # https://tools.ietf.org/html/rfc4566#section-5: "The sequence # CRLF
        # (0x0d0a) is used to end a record, although parsers SHOULD be tolerant
        # and also accept records terminated with a single newline character."
        # We represent the test cases with LF line endings for convenience, and
        # test them both that way and with CRLF line endings.
        expect(Parse.ipFromSDP(test.sdp)?.toLowerCase()).toEqual(test.expected)
        expect(Parse.ipFromSDP(test.sdp.replace(/\n/, "\r\n"))?.toLowerCase()).toEqual(test.expected)

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
  # Fake snowflake to interact with
  snowflake = {
    broker:
      sendAnswer: ->
  }
  pp = new ProxyPair(fakeRelay, rateLimit)

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
