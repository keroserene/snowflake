###
All of Snowflake's DOM manipulation and inputs.
###

class UI
  setStatus: (msg) ->

  setActive: (connected) ->

  log: (msg) ->


class BadgeUI extends UI
  $badge: null

  constructor: ->
    @$badge = document.getElementById('badge')

  setActive: (connected) =>
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
  setStatus: (msg) =>
    @$status.innerHTML = 'Status: ' + msg

  setActive: (connected) =>
    @$msglog.className = if connected then 'active' else ''

  log: (msg) =>
    # Scroll to latest
    @$msglog.value += msg + '\n'
    @$msglog.scrollTop = @$msglog.scrollHeight


class WebExtUI extends UI
  setActive: (connected) ->
    chrome.browserAction.setIcon {
      "path": {
        "32": "icons/status-" + (if connected then "on" else "off") + ".png"
      }
    }
