###
A Coffeescript WebRTC snowflake proxy

Contains helpers for parsing query strings and other utilities.
###


Query =
  ###
  Parse a URL query string or application/x-www-form-urlencoded body. The
  return type is an object mapping string keys to string values. By design,
  this function doesn't support multiple values for the same named parameter,
  for example 'a=1&a=2&a=3'; the first definition always wins. Returns null on
  error.

  Always decodes from UTF-8, not any other encoding.
  http://dev.w3.org/html5/spec/Overview.html#url-encoded-form-data
  ###
  parse: (qs) ->
    result = {}
    strings = []
    strings = qs.split '&' if qs
    return result if 0 == strings.length
    for string in strings
      j = string.indexOf '='
      if j == -1
        name = string
        value = ''
      else
        name = string.substr(0, j)
        value = string.substr(j + 1)
      name = decodeURIComponent(name.replace(/\+/g, ' '))
      value = decodeURIComponent(value.replace(/\+/g, ' '))
      result[name] = value if name not of result
    result

  # params is a list of (key, value) 2-tuples.
  buildString: (params) ->
    parts = []
    for param in params
      parts.push encodeURIComponent(param[0]) + '=' +
                 encodeURIComponent(param[1])
    parts.join '&'


Parse =
  # Parse a cookie data string (usually document.cookie). The return type is an
  # object mapping cookies names to values. Returns null on error.
  # http://www.w3.org/TR/DOM-Level-2-HTML/html.html#ID-8747038
  cookie: (cookies) ->
    result = {}
    strings = []
    strings = cookies.split ';' if cookies
    for string in strings
      j = string.indexOf '='
      return null if -1 == j
      name  = decodeURIComponent string.substr(0, j).trim()
      value = decodeURIComponent string.substr(j + 1).trim()
      result[name] = value if !(name in result)
    result

  # Parse an address in the form 'host:port'. Returns an Object with keys 'host'
  # (String) and 'port' (int). Returns null on error.
  address: (spec) ->
    m = null
    # IPv6 syntax.
    m = spec.match(/^\[([\0-9a-fA-F:.]+)\]:([0-9]+)$/) if !m
    # IPv4 syntax.
    m = spec.match(/^([0-9.]+):([0-9]+)$/) if !m
    return null if !m

    host = m[1]
    port = parseInt(m[2], 10)
    if isNaN(port) || port < 0 || port > 65535
      return null
    { host: host, port: port }

  # Parse a count of bytes. A suffix of 'k', 'm', or 'g' (or uppercase)
  # does what you would think. Returns null on error.
  byteCount: (spec) ->
    UNITS = {
      k: 1024, m: 1024 * 1024, g: 1024 * 1024 * 1024
      K: 1024, M: 1024 * 1024, G: 1024 * 1024 * 1024
    }
    matches = spec.match /^(\d+(?:\.\d*)?)(\w*)$/
    return null if null == matches
    count = Number matches[1]
    return null if isNaN count
    if '' == matches[2]
      units = 1
    else
      units = UNITS[matches[2]]
      return null if null == units
    count * Number(units)


Params =
  getBool: (query, param, defaultValue) ->
    val = query[param]
    return defaultValue if undefined == val
    return true if 'true' == val || '1' == val || '' == val
    return false if 'false' == val || '0' == val
    return null

  # Get an object value and parse it as a byte count. Example byte counts are
  # '100' and '1.3m'. Returns |defaultValue| if param is not a key. Return null
  # on a parsing error.
  getByteCount: (query, param, defaultValue) ->
    spec = query[param]
    return defaultValue if undefined == spec
    Parse.byteCount spec

  # Get an object value and parse it as an address spec. Returns |defaultValue|
  # if param is not a key. Returns null on a parsing error.
  getAddress: (query, param, defaultValue) ->
    val = query[param]
    return defaultValue if undefined == val
    Parse.address val

  # Get an object value and return it as a string. Returns default_val if param
  # is not a key.
  getString: (query, param, defaultValue) ->
    val = query[param]
    return defaultValue if undefined == val
    val

class BucketRateLimit
  amount: 0.0
  lastUpdate: new Date()

  constructor: (@capacity, @time) ->

  age: ->
    now = new Date()
    delta = (now - @lastUpdate) / 1000.0
    @lastUpdate = now
    @amount -= delta * @capacity / @time
    @amount = 0.0 if @amount < 0.0

  update: (n) ->
    @age()
    @amount += n
    @amount <= @capacity

  # How many seconds in the future will the limit expire?
  when: ->
    age()
    (@amount - @capacity) / (@capacity / @time)

  isLimited: ->
    @age()
    @amount > @capacity


# A rate limiter that never limits.
class DummyRateLimit
  constructor: (@capacity, @time) ->
  update: (n) -> true
  when: -> 0.0
  isLimited: -> false
