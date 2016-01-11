# s = require './snowflake'

window = {}

VERBOSE = false
if process.argv.indexOf('-v') >= 0
    VERBOSE = true
numTests = 0
numFailed = 0

announce = (testName) ->
  if VERBOSE
    console.log '\n --- ' + testName + ' ---'

pass = (test) ->
  numTests++;
  if VERBOSE
    console.log 'PASS ' + test

fail = (test, expected, actual) ->
  numTests++
  numFailed++
  console.log 'FAIL ' + test +
    '  expected: ' + JSON.stringify(expected) +
    '  actual: ' + JSON.stringify(actual)


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
      expected: { }
    },{
      cs: 'a=b'
      expected: { a: 'b'}
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
      expected: {a: 'b' }
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
    actual = Params.parseCookie test.cs
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


testBuildUrl()
testParseCookieString()
testParseQueryString()
