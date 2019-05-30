###
All of Snowflake's DOM manipulation and inputs.
###

class UI
  active: false

  setStatus: (msg) ->

  setActive: (connected) ->
    @active = connected

  log: (msg) ->


class BadgeUI extends UI
  $badge: null

  constructor: ->
    @$badge = document.getElementById('badge')

  setActive: (connected) ->
    super connected
    @$badge.className = if connected then 'active' else ''


class DebugUI extends UI
  # DOM elements references.
  $msglog: null
  $status: null

  constructor: ->
    # Setup other DOM handlers if it's debug mode.
    @$status = document.getElementById('status')
    @$msglog = document.getElementById('msglog')
    @$msglog.value = ''

  # Status bar
  setStatus: (msg) ->
    @$status.innerHTML = 'Status: ' + msg

  setActive: (connected) ->
    super connected
    @$msglog.className = if connected then 'active' else ''

  log: (msg) ->
    # Scroll to latest
    @$msglog.value += msg + '\n'
    @$msglog.scrollTop = @$msglog.scrollHeight


class WebExtUI extends UI
  port: null

  constructor: ->
    chrome.runtime.onConnect.addListener @onConnect

  postActive: ->
    @port?.postMessage
      active: @active

  onConnect: (port) =>
    @port = port
    port.onDisconnect.addListener @onDisconnect
    @postActive()

  onDisconnect: (port) =>
    @port = null

  setActive: (connected) ->
    super connected
    @postActive()
    chrome.browserAction.setIcon
      path:
        32: "icons/status-" + (if connected then "on" else "off") + ".png"
