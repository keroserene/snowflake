
# Fake snowflake to interact with
snowflake =
  ui: new UI
  broker:
    sendAnswer: ->
  state: Snowflake.MODE.INIT

describe 'Init', ->

  it 'gives a dialog when closing, only while active', ->
    silenceNotifications = false
    snowflake.state = Snowflake.MODE.WEBRTC_READY
    msg = window.onbeforeunload()
    expect(snowflake.state).toBe Snowflake.MODE.WEBRTC_READY
    expect(msg).toBe Snowflake.MESSAGE.CONFIRMATION

    snowflake.state = Snowflake.MODE.INIT
    msg = window.onbeforeunload()
    expect(snowflake.state).toBe Snowflake.MODE.INIT
    expect(msg).toBe null

  it 'does not give a dialog when silent flag is on', ->
    silenceNotifications = true
    snowflake.state = Snowflake.MODE.WEBRTC_READY
    msg = window.onbeforeunload()
    expect(snowflake.state).toBe Snowflake.MODE.WEBRTC_READY
    expect(msg).toBe null
