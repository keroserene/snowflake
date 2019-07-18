/* global expect, it, describe, Parse, Query, Params */

/*
jasmine tests for Snowflake utils
*/

describe('Parse', function() {

  describe('cookie', function() {

    it('parses correctly', function() {
      expect(Parse.cookie('')).toEqual({});
      expect(Parse.cookie('a=b')).toEqual({
        a: 'b'
      });
      expect(Parse.cookie('a=b=c')).toEqual({
        a: 'b=c'
      });
      expect(Parse.cookie('a=b; c=d')).toEqual({
        a: 'b',
        c: 'd'
      });
      expect(Parse.cookie('a=b ; c=d')).toEqual({
        a: 'b',
        c: 'd'
      });
      expect(Parse.cookie('a= b')).toEqual({
        a: 'b'
      });
      expect(Parse.cookie('a=')).toEqual({
        a: ''
      });
      expect(Parse.cookie('key')).toBeNull();
      expect(Parse.cookie('key=%26%20')).toEqual({
        key: '& '
      });
      expect(Parse.cookie('a=\'\'')).toEqual({
        a: '\'\''
      });
    });

  });

  describe('address', function() {

    it('parses IPv4', function() {
      expect(Parse.address('')).toBeNull();
      expect(Parse.address('3.3.3.3:4444')).toEqual({
        host: '3.3.3.3',
        port: 4444
      });
      expect(Parse.address('3.3.3.3')).toBeNull();
      expect(Parse.address('3.3.3.3:0x1111')).toBeNull();
      expect(Parse.address('3.3.3.3:-4444')).toBeNull();
      expect(Parse.address('3.3.3.3:65536')).toBeNull();
    });

    it('parses IPv6', function() {
      expect(Parse.address('[1:2::a:f]:4444')).toEqual({
        host: '1:2::a:f',
        port: 4444
      });
      expect(Parse.address('[1:2::a:f]')).toBeNull();
      expect(Parse.address('[1:2::a:f]:0x1111')).toBeNull();
      expect(Parse.address('[1:2::a:f]:-4444')).toBeNull();
      expect(Parse.address('[1:2::a:f]:65536')).toBeNull();
      expect(Parse.address('[1:2::ffff:1.2.3.4]:4444')).toEqual({
        host: '1:2::ffff:1.2.3.4',
        port: 4444
      });
    });

  });

  describe('ipFromSDP', function() {

    var testCases = [
      {
        // https://tools.ietf.org/html/rfc4566#section-5
        sdp: "v=0\no=jdoe 2890844526 2890842807 IN IP4 10.47.16.5\ns=SDP Seminar\ni=A Seminar on the session description protocol\nu=http://www.example.com/seminars/sdp.pdf\ne=j.doe@example.com (Jane Doe)\nc=IN IP4 224.2.17.12/127\nt=2873397496 2873404696\na=recvonly\nm=audio 49170 RTP/AVP 0\nm=video 51372 RTP/AVP 99\na=rtpmap:99 h263-1998/90000",
        expected: '224.2.17.12'
      },
      {
        // Missing c= line
        sdp: "v=0\no=jdoe 2890844526 2890842807 IN IP4 10.47.16.5\ns=SDP Seminar\ni=A Seminar on the session description protocol\nu=http://www.example.com/seminars/sdp.pdf\ne=j.doe@example.com (Jane Doe)\nt=2873397496 2873404696\na=recvonly\nm=audio 49170 RTP/AVP 0\nm=video 51372 RTP/AVP 99\na=rtpmap:99 h263-1998/90000",
        expected: void 0
      },
      {
        // Single line, IP address only
        sdp: "c=IN IP4 224.2.1.1\n",
        expected: '224.2.1.1'
      },
      {
        // Same, with TTL
        sdp: "c=IN IP4 224.2.1.1/127\n",
        expected: '224.2.1.1'
      },
      {
        // Same, with TTL and multicast addresses
        sdp: "c=IN IP4 224.2.1.1/127/3\n",
        expected: '224.2.1.1'
      },
      {
        // IPv6, address only
        sdp: "c=IN IP6 FF15::101\n",
        expected: 'ff15::101'
      },
      {
        // Same, with multicast addresses
        sdp: "c=IN IP6 FF15::101/3\n",
        expected: 'ff15::101'
      },
      {
        // Multiple c= lines
        sdp: "c=IN IP4 1.2.3.4\nc=IN IP4 5.6.7.8",
        expected: '1.2.3.4'
      },
      {
        // Modified from SDP sent by snowflake-client.
        sdp: "v=0\no=- 7860378660295630295 2 IN IP4 127.0.0.1\ns=-\nt=0 0\na=group:BUNDLE data\na=msid-semantic: WMS\nm=application 54653 DTLS/SCTP 5000\nc=IN IP4 1.2.3.4\na=candidate:3581707038 1 udp 2122260223 192.168.0.1 54653 typ host generation 0 network-id 1 network-cost 50\na=candidate:2617212910 1 tcp 1518280447 192.168.0.1 59673 typ host tcptype passive generation 0 network-id 1 network-cost 50\na=candidate:2082671819 1 udp 1686052607 1.2.3.4 54653 typ srflx raddr 192.168.0.1 rport 54653 generation 0 network-id 1 network-cost 50\na=ice-ufrag:IBdf\na=ice-pwd:G3lTrrC9gmhQx481AowtkhYz\na=fingerprint:sha-256 53:F8:84:D9:3C:1F:A0:44:AA:D6:3C:65:80:D3:CB:6F:23:90:17:41:06:F9:9C:10:D8:48:4A:A8:B6:FA:14:A1\na=setup:actpass\na=mid:data\na=sctpmap:5000 webrtc-datachannel 1024",
        expected: '1.2.3.4'
      },
      {
        // Improper character within IPv4
        sdp: "c=IN IP4 224.2z.1.1",
        expected: void 0
      },
      {
        // Improper character within IPv6
        sdp: "c=IN IP6 ff15:g::101",
        expected: void 0
      },
      {
        // Bogus "IP7" addrtype
        sdp: "c=IN IP7 1.2.3.4\n",
        expected: void 0
      }
    ];

    it('parses SDP', function() {
      var i, len, ref, ref1, results, test;
      results = [];
      for (i = 0, len = testCases.length; i < len; i++) {
        test = testCases[i];
        // https://tools.ietf.org/html/rfc4566#section-5: "The sequence # CRLF
        // (0x0d0a) is used to end a record, although parsers SHOULD be tolerant
        // and also accept records terminated with a single newline character."
        // We represent the test cases with LF line endings for convenience, and
        // test them both that way and with CRLF line endings.
        expect((ref = Parse.ipFromSDP(test.sdp)) != null ? ref.toLowerCase() : void 0).toEqual(test.expected);
        results.push(expect((ref1 = Parse.ipFromSDP(test.sdp.replace(/\n/, "\r\n"))) != null ? ref1.toLowerCase() : void 0).toEqual(test.expected));
      }
      return results;
    });

  });

});

describe('query string', function() {

  it('should parse correctly', function() {
    expect(Query.parse('')).toEqual({});
    expect(Query.parse('a=b')).toEqual({
      a: 'b'
    });
    expect(Query.parse('a=b=c')).toEqual({
      a: 'b=c'
    });
    expect(Query.parse('a=b&c=d')).toEqual({
      a: 'b',
      c: 'd'
    });
    expect(Query.parse('client=&relay=1.2.3.4%3A9001')).toEqual({
      client: '',
      relay: '1.2.3.4:9001'
    });
    expect(Query.parse('a=b%26c=d')).toEqual({
      a: 'b&c=d'
    });
    expect(Query.parse('a%3db=d')).toEqual({
      'a=b': 'd'
    });
    expect(Query.parse('a=b+c%20d')).toEqual({
      'a': 'b c d'
    });
    expect(Query.parse('a=b+c%2bd')).toEqual({
      'a': 'b c+d'
    });
    expect(Query.parse('a+b=c')).toEqual({
      'a b': 'c'
    });
    expect(Query.parse('a=b+c+d')).toEqual({
      a: 'b c d'
    });
  });

  it('uses the first appearance of duplicate key', function() {
    expect(Query.parse('a=b&c=d&a=e')).toEqual({
      a: 'b',
      c: 'd'
    });
    expect(Query.parse('a')).toEqual({
      a: ''
    });
    expect(Query.parse('=b')).toEqual({
      '': 'b'
    });
    expect(Query.parse('&a=b')).toEqual({
      '': '',
      a: 'b'
    });
    expect(Query.parse('a=b&')).toEqual({
      a: 'b',
      '': ''
    });
    expect(Query.parse('a=b&&c=d')).toEqual({
      a: 'b',
      '': '',
      c: 'd'
    });
  });

});

describe('Params', function() {

  describe('bool', function() {

    var getBool = function(query) {
      return Params.getBool(Query.parse(query), 'param', false);
    };

    it('parses correctly', function() {
      expect(getBool('param=true')).toBe(true);
      expect(getBool('param')).toBe(true);
      expect(getBool('param=')).toBe(true);
      expect(getBool('param=1')).toBe(true);
      expect(getBool('param=0')).toBe(false);
      expect(getBool('param=false')).toBe(false);
      expect(getBool('param=unexpected')).toBeNull();
      expect(getBool('pram=true')).toBe(false);
    });

  });

});
