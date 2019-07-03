###
Entry point.
###

if (not TESTING? or not TESTING) and not Util.featureDetect()
  console.log 'webrtc feature not detected. shutting down'
  return

snowflake = null

query = Query.parse(location)
debug = Params.getBool(query, 'debug', false)
silenceNotifications = Params.getBool(query, 'silent', false)

# Log to both console and UI if applicable.
# Requires that the snowflake and UI objects are hooked up in order to
# log to console.
log = (msg) ->
  console.log 'Snowflake: ' + msg
  snowflake?.ui.log msg

dbg = (msg) -> log msg if debug or (snowflake?.ui instanceof DebugUI)

init = () ->
  config = new Config

  if 'off' != query['ratelimit']
    config.rateLimitBytes = Params.getByteCount(
      query,'ratelimit', config.rateLimitBytes
    )

  ui = null
  if (document.getElementById('badge') != null)
    ui = new BadgeUI()
  else if (document.getElementById('status') != null)
    ui = new DebugUI()
  else
    ui = new UI()

  broker = new Broker config.brokerUrl
  snowflake = new Snowflake config, ui, broker

  log '== snowflake proxy =='
  if Util.snowflakeIsDisabled(config.cookieName)
    # Do not activate the proxy if any number of conditions are true.
    log 'Currently not active.'
    return

  # Otherwise, begin setting up WebRTC and acting as a proxy.
  dbg 'Contacting Broker at ' + broker.url
  snowflake.setRelayAddr config.relayAddr
  snowflake.beginWebRTC()

# Notification of closing tab with active proxy.
window.onbeforeunload = ->
  if !silenceNotifications && Snowflake.MODE.WEBRTC_READY == snowflake.state
    return Snowflake.MESSAGE.CONFIRMATION
  null

window.onunload = ->
  snowflake.disable()
  null

window.onload = init
