###
All of Snowflake's DOM manipulation and inputs.
###

class UI
  debug = false  # True when there's no badge

  # DOM elements references.
  $msglog: null
  $send:   null
  $input:  null
  $status: null

  constructor: ->
    @$badge = document.getElementById('badge')
    @debug = null == @$badge
    return if !@debug

    # Setup other DOM handlers if it's debug mode.
    @$status = document.getElementById('status')
    @$msglog = document.getElementById('msglog')
    @$msglog.value = ''

    @$send = document.getElementById('send')
    @$send.onclick = @acceptInput

    @$input = document.getElementById('input')
    @$input.focus()
    @$input.onkeydown = (e) =>
      @$send.onclick() if 13 == e.keyCode  # enter

  # Status bar
  setStatus: (msg) =>
    return if !@debug
    @$status.innerHTML = 'Status: ' + msg

  setActive: (connected) =>
    if @debug
      @$msglog.className = if connected then 'active' else ''
    else
      @$badge.className = if connected then 'active' else ''

  # Local input from keyboard into message window.
  acceptInput: =>
    msg = @$input.value
    if !COPY_PASTE_ENABLED
      @log 'No input expected - Copy Paste Signalling disabled.'
    else switch snowflake.state
      when MODE.WEBRTC_CONNECTING
        Signalling.receive msg
      when MODE.WEBRTC_READY
        @log 'No input expected - WebRTC connected.'
      else
        @log 'ERROR: ' + msg
    @$input.value = ''
    @$input.focus()

  log: (msg) =>
    return if !@debug
    # Scroll to latest
    @$msglog.value += msg + '\n'
    @$msglog.scrollTop = @$msglog.scrollHeight
