###
Only websocket-specific stuff.
###

WSS_ENABLED = true
DEFAULT_PORTS =
  http:  80
  https: 443

# Build an escaped URL string from unescaped components. Only scheme and host
# are required. See RFC 3986, section 3.
buildUrl = (scheme, host, port, path, params) ->
  parts = []
  parts.push(encodeURIComponent scheme)
  parts.push '://'

  # If it contains a colon but no square brackets, treat it as IPv6.
  if host.match(/:/) && !host.match(/[[\]]/)
    parts.push '['
    parts.push host
    parts.push ']'
  else
    parts.push(encodeURIComponent host)

  if undefined != port && DEFAULT_PORTS[scheme] != port
    parts.push ':'
    parts.push(encodeURIComponent port.toString())

  if undefined != path && '' != path
    if !path.match(/^\//)
      path = '/' + path
    ###
    Slash is significant so we must protect it from encodeURIComponent, while
    still encoding question mark and number sign. RFC 3986, section 3.3: 'The
    path is terminated by the first question mark ('?') or number sign ('#')
    character, or by the end of the URI. ... A path consists of a sequence of
    path segments separated by a slash ('/') character.'
    ###
    path = path.replace /[^\/]+/, (m) ->
      encodeURIComponent m
    parts.push path

  if undefined != params
    parts.push '?'
    parts.push Query.buildString params

  parts.join ''

makeWebsocket = (addr) ->
  wsProtocol = if WSS_ENABLED then 'wss' else 'ws'
  url = buildUrl wsProtocol, addr.host, addr.port, '/'
  ws = new WebSocket url
  ###
  'User agents can use this as a hint for how to handle incoming binary data: if
  the attribute is set to 'blob', it is safe to spool it to disk, and if it is
  set to 'arraybuffer', it is likely more efficient to keep the data in memory.'
  ###
  ws.binaryType = 'arraybuffer'
  ws
