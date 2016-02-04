window = {}
ui = {}

VERBOSE = false
VERBOSE = true if process.argv.indexOf('-v') >= 0

numTests = 0
numFailed = 0

announce = (testName) ->
  console.log '\n --- ' + testName + ' ---' if VERBOSE

pass = (test) ->
  numTests++
  console.log 'PASS ' + test if VERBOSE

fail = (test, expected, actual) ->
  numTests++
  numFailed++
  console.log 'FAIL ' + test +
    '  expected: ' + JSON.stringify(expected) +
    '  actual: ' + JSON.stringify(actual)

# Stubs for browser functionality.
class WebSocket
  OPEN: 1
  CLOSED: 0

testBuildUrl = ->
  TESTS = [{
      args: ['http', 'example.com']
      expected: 'http://example.com'
    },{
      args: ['http', 'example.com', 80]
      expected: 'http://example.com'
    },{
      args: ['http', 'example.com', 81],
      expected: 'http://example.com:81'
    },{
      args: ['https', 'example.com', 443]
      expected: 'https://example.com'
    },{
      args: ['https', 'example.com', 444]
      expected: 'https://example.com:444'
    },{
      args: ['http', 'example.com', 80, '/']
      expected: 'http://example.com/'
    },{
      args: ['http', 'example.com', 80, '/test?k=%#v']
      expected: 'http://example.com/test%3Fk%3D%25%23v'
    },{
      args: ['http', 'example.com', 80, '/test', []]
      expected: 'http://example.com/test?'
    },{
      args: ['http', 'example.com', 80, '/test', [['k', '%#v']]]
      expected: 'http://example.com/test?k=%25%23v'
    },{
      args: ['http', 'example.com', 80, '/test', [['a', 'b'], ['c', 'd']]]
      expected: 'http://example.com/test?a=b&c=d'
    },{
      args: ['http', '1.2.3.4']
      expected: 'http://1.2.3.4'
    },{
      args: ['http', '1:2::3:4']
      expected: 'http://[1:2::3:4]'
    },{
      args: ['http', 'bog][us']
      expected: 'http://bog%5D%5Bus'
    },{
      args: ['http', 'bog:u]s']
      expected: 'http://bog%3Au%5Ds'
    }]
  announce 'testBuildUrl'
  for test in TESTS
    actual = buildUrl.apply undefined, test.args
    if actual == test.expected
      pass test.args
    else
      fail test.args, test.expected, actual

###
This test only checks that things work for strings formatted like
document.cookie. Browsers maintain several properties about this string, for
example cookie names are unique with no trailing whitespace.  See
http://www.ietf.org/rfc/rfc2965.txt for the grammar.
###
testParseCookieString = ->
  TESTS = [{
      cs: ''
      expected: {}
    },{
      cs: 'a=b'
      expected: { a: 'b' }
    },{
      cs: 'a=b=c'
      expected: { a: 'b=c' }
    },{
      cs: 'a=b; c=d'
      expected: { a: 'b', c: 'd' }
    },{
      cs: 'a=b ; c=d'
      expected: { a: 'b', c: 'd' }
    },{
      cs: 'a= b',
      expected: { a: 'b' }
    },{
      cs: 'a='
      expected: { a: '' }
    }, {
      cs: 'key',
      expected: null
    }, {
      cs: 'key=%26%20'
      expected: { key: '& ' }
    }, {
      cs: 'a=\'\''
      expected: { a: '\'\'' }
    }]
  announce 'testParseCookieString'
  for test in TESTS
    actual = Parse.cookie test.cs
    if JSON.stringify(actual) == JSON.stringify(test.expected)
      pass test.cs
    else
      fail test.cs, test.expected, actual

testParseQueryString = ->
  TESTS = [{
      qs: ''
      expected: {}
    },{
      qs: 'a=b'
      expected: { a: 'b' }
    },{
      qs: 'a=b=c'
      expected: { a: 'b=c' }
    },{
      qs: 'a=b&c=d'
      expected: { a: 'b', c: 'd' }
    },{
      qs: 'client=&relay=1.2.3.4%3A9001'
      expected: { client: '', relay: '1.2.3.4:9001' }
    },{
      qs: 'a=b%26c=d'
      expected: { a: 'b&c=d' }
    },{
      qs: 'a%3db=d'
      expected: { 'a=b': 'd' }
    },{
      qs: 'a=b+c%20d'
      expected: { 'a': 'b c d' }
    },{
      qs: 'a=b+c%2bd'
      expected: { 'a': 'b c+d' }
    },{
      qs: 'a+b=c'
      expected: { 'a b': 'c' }
    },{
      qs: 'a=b+c+d'
      expected: { a: 'b c d' }
    # First appearance wins.
    },{
      qs: 'a=b&c=d&a=e'
      expected: { a: 'b', c: 'd' }
    },{
      qs: 'a'
      expected: { a: '' }
    },{
      qs: '=b',
      expected: { '': 'b' }
    },{
      qs: '&a=b'
      expected: { '': '', a: 'b' }
    },{
      qs: 'a=b&'
      expected: { a: 'b', '':'' }
    },{
      qs: 'a=b&&c=d'
      expected: { a: 'b', '':'', c: 'd' }
    }]
  announce 'testParseQueryString'
  for test in TESTS
    actual = Query.parse test.qs
    if JSON.stringify(actual) == JSON.stringify(test.expected)
      pass test.qs
    else
      fail test.qs, test.expected, actual

testGetParamBoolean = ->
  TESTS = [{
      qs: 'param=true'
      expected: true
    },{
      qs: 'param',
      expected: true
    },{
      qs: 'param='
      expected: true
    },{
      qs: 'param=1'
      expected: true
    },{
      qs: 'param=0'
      expected: false
    },{
      qs: 'param=false'
      expected: false
    },{
      qs: 'param=unexpected'
      expected: null
    },{
      qs: 'pram=true'
      expected: false
    }]
  announce 'testGetParamBoolean'
  for test in TESTS
    query = Query.parse test.qs
    actual = Params.getBool(query, 'param', false)
    if actual == test.expected
      pass test.qs
    else
      fail test.qs, test.expected, actual

testParseAddress = ->
  TESTS = [{
      spec: ''
      expected: null
    },{
      spec: '3.3.3.3:4444'
      expected: { host: '3.3.3.3', port: 4444 }
    },{
      spec: '3.3.3.3'
      expected: null
    },{
      spec: '3.3.3.3:0x1111'
      expected: null
    },{
      spec: '3.3.3.3:-4444'
      expected: null
    },{
      spec: '3.3.3.3:65536'
      expected: null
    },{
      spec: '[1:2::a:f]:4444'
      expected: { host: '1:2::a:f', port: 4444 }
    },{
      spec: '[1:2::a:f]'
      expected: null
    },{
      spec: '[1:2::a:f]:0x1111'
      expected: null
    },{
      spec: '[1:2::a:f]:-4444'
      expected: null
    },{
      spec: '[1:2::a:f]:65536'
      expected: null
    },{
      spec: '[1:2::ffff:1.2.3.4]:4444'
      expected: { host: '1:2::ffff:1.2.3.4', port: 4444 }
    }]
  announce 'testParseAddrSpec'
  for test in TESTS
    actual = Parse.address test.spec
    if JSON.stringify(actual) == JSON.stringify(test.expected)
      pass test.spec
    else
      fail test.spec, test.expected, actual

testGetParamAddress = ->
  DEFAULT = { host: '1.1.1.1', port: 2222 }
  TESTS = [{
      query: {}
      expected: DEFAULT
    },{
      query: { addr: '3.3.3.3:4444' },
      expected: { host: '3.3.3.3', port: 4444 }
    },{
      query: { x: '3.3.3.3:4444' }
      expected: DEFAULT
    },{
      query: { addr: '---' }
      expected: null
    }]

  announce 'testGetParamAddress'
  for test in TESTS
    actual = Params.getAddress test.query, 'addr', DEFAULT
    if JSON.stringify(actual) == JSON.stringify(test.expected)
      pass test.query
    else
      fail test.query, test.expected, actual

testProxyPair = ->
  announce 'testProxyPair'
  fakeRelay = Parse.address '0.0.0.0:12345'
  rateLimit = new DummyRateLimit()
  destination = []
  fakeClient =
    send: (d) -> destination.push d
  pp = new ProxyPair(fakeClient, fakeRelay, rateLimit)
  pp.connectRelay()
  if null != pp.relay.onopen then pass 'relay.onopen'
  else fail 'relay onopen must not be null.'
  if null != pp.relay.onclose then pass 'relay.onclose'
  else fail 'relay onclose must not be null.'
  if null != pp.relay.onerror then pass 'relay.onerror'
  else fail 'relay onerror must not be null.'
  if null != pp.relay.onmessage then pass 'relay.onmessage'
  else fail 'relay onmessage must not be null.'
  # TODO: Test for flush
  # pp.c2rSchedule.push { data: 'omg' }
  # pp.flush()
  # if destination == ['omg'] then pass 'flush'
  # else fail 'flush', ['omg'], destination

testBuildUrl()
testParseCookieString()
testParseQueryString()
testGetParamBoolean()
testParseAddress()
testGetParamAddress()
testProxyPair()
