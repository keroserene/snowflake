###
Entry point.
###

debug = false
snowflake = null
config = null
broker = null
ui = null

# Log to both console and UI if applicable.
# Requires that the snowflake and UI objects are hooked up in order to
# log to console.
log = (msg) ->
  console.log 'Snowflake: ' + msg
  snowflake?.ui.log msg

dbg = (msg) -> log msg if debug

init = () ->
  config = new Config
  ui = new WebExtUI()
  broker = new Broker config.brokerUrl
  snowflake = new Snowflake config, ui, broker

  log '== snowflake proxy =='

update = () ->
  if !ui.enabled
    # Do not activate the proxy if any number of conditions are true.
    snowflake.disable()
    log 'Currently not active.'
    return

  # Otherwise, begin setting up WebRTC and acting as a proxy.
  dbg 'Contacting Broker at ' + broker.url
  log 'Starting snowflake'
  snowflake.setRelayAddr config.relayAddr
  snowflake.beginWebRTC()

# Notification of closing tab with active proxy.
window.onbeforeunload = ->
  if !silenceNotifications && Snowflake.MODE.WEBRTC_READY == snowflake.state
    return Snowflake.MESSAGE.CONFIRMATION
  null

window.onunload = ->
  pair.close() for pair in snowflake.proxyPairs
  null

window.onload = init
