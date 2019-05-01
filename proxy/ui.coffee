###
All of Snowflake's DOM manipulation and inputs.
###

class UI
  debug: false  # True when there's no badge

  # DOM elements references.
  $msglog: null
  $status: null

  constructor: ->
    @$badge = document.getElementById('badge')
    @debug = null == @$badge
    return if !@debug

    # Setup other DOM handlers if it's debug mode.
    @$status = document.getElementById('status')
    @$msglog = document.getElementById('msglog')
    @$msglog.value = ''

  # Status bar
  setStatus: (msg) =>
    return if !@debug
    @$status.innerHTML = 'Status: ' + msg

  setActive: (connected) =>
    if @debug
      @$msglog.className = if connected then 'active' else ''
    else
      @$badge.className = if connected then 'active' else ''

  log: (msg) =>
    return if !@debug
    # Scroll to latest
    @$msglog.value += msg + '\n'
    @$msglog.scrollTop = @$msglog.scrollHeight
