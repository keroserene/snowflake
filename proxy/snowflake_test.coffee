s = require './snowflake'

VERBOSE = false
if process.argv.indexOf("-v") >= 0
    VERBOSE = true
numTests = 0
numFailed = 0

announce = (testName) ->
  if VERBOSE
    # if (!top)
      # console.log();
    console.log testName
  # top = false

pass = (test) ->
  numTests++;
  if VERBOSE
    console.log "PASS " + test

fail = (test, expected, actual) ->
  numTests++
  numFailed++
  console.log "FAIL " + test + "  expected: " + expected + "  actual: " + actual


testBuildUrl = ->
  TESTS = [{
      args: ["http", "example.com"]
      expected: "http://example.com"
    },{
      args: ["http", "example.com", 80]
      expected: "http://example.com"
    },{
      args: ["http", "example.com", 81],
      expected: "http://example.com:81"
    },{
      args: ["https", "example.com", 443]
      expected: "https://example.com"
    },{
      args: ["https", "example.com", 444]
      expected: "https://example.com:444"
    },{
      args: ["http", "example.com", 80, "/"]
      expected: "http://example.com/"
    },{
      args: ["http", "example.com", 80, "/test?k=%#v"]
      expected: "http://example.com/test%3Fk%3D%25%23v"
    },{
      args: ["http", "example.com", 80, "/test", []]
      expected: "http://example.com/test?"
    },{
      args: ["http", "example.com", 80, "/test", [["k", "%#v"]]]
      expected: "http://example.com/test?k=%25%23v"
    },{
      args: ["http", "example.com", 80, "/test", [["a", "b"], ["c", "d"]]]
      expected: "http://example.com/test?a=b&c=d"
    },{
      args: ["http", "1.2.3.4"]
      expected: "http://1.2.3.4"
    },{
      args: ["http", "1:2::3:4"]
      expected: "http://[1:2::3:4]"
    },{
      args: ["http", "bog][us"]
      expected: "http://bog%5D%5Bus"
    },{
      args: ["http", "bog:u]s"]
      expected: "http://bog%3Au%5Ds"
    }
  ]
  announce "-- testBuildUrl --"
  for test in TESTS
    actual = s.buildUrl.apply undefined, test.args
    if actual == test.expected
      pass test.args
    else
      fail test.args, test.expected, actual

testBuildUrl()
