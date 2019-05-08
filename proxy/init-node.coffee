###
Entry point.
###

config = new Config
ui = new UI()
broker = new Broker config.brokerUrl
snowflake = new Snowflake config, ui, broker

log = (msg) ->
  console.log 'Snowflake: ' + msg

dbg = log

log '== snowflake proxy =='
dbg 'Contacting Broker at ' + broker.url

snowflake.setRelayAddr config.relayAddr
snowflake.beginWebRTC()
