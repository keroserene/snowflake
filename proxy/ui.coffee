###
All of Snowflake's DOM manipulation and inputs.
###

class UI
  active: false
  enabled: false

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
    txt = document.createTextNode('Status: ' + msg)
    while @$status.firstChild
      @$status.removeChild @$status.firstChild
    @$status.appendChild txt

  setActive: (connected) ->
    super connected
    @$msglog.className = if connected then 'active' else ''

  log: (msg) ->
    # Scroll to latest
    @$msglog.value += msg + '\n'
    @$msglog.scrollTop = @$msglog.scrollHeight


class WebExtUI extends UI
  port: null
  stats: null

  constructor: ->
    @initStats()
    @initToggle()
    chrome.runtime.onConnect.addListener @onConnect

  initStats: ->
    @stats = [0]
    setInterval (() =>
      @stats.unshift 0
      @stats.splice 24
      @postActive()
    ), 60 * 60 * 1000

  initToggle: ->
    try
      getting = chrome.storage.local.get("snowflake-enabled", (result) => 
        @enabled = result['snowflake-enabled']
        update()
        chrome.browserAction.setIcon
          path:
            32: "icons/status-" + (if @enabled then "on" else "off") + ".png"
      )
    catch
      log "Toggle state not yet saved"

  postActive: ->
    @port?.postMessage
      active: @active
      total: @stats.reduce ((t, c) ->
        t + c
      ), 0
      enabled: @enabled

  onConnect: (port) =>
    @port = port
    port.onDisconnect.addListener @onDisconnect
    port.onMessage.addListener @onMessage
    @postActive()

  onMessage: (m) =>
    @enabled = m.enabled
    update()
    chrome.browserAction.setIcon
      path:
        32: "icons/status-" + (if @enabled then "on" else "off") + ".png"
    @postActive()
    storing = chrome.storage.local.set({"snowflake-enabled":@enabled}, () -> log "Stored toggle state")

  onDisconnect: (port) =>
    @port = null

  setActive: (connected) ->
    super connected
    if connected then @stats[0] += 1
    @postActive()
    if @active
      chrome.browserAction.setIcon
        path:
          32: "icons/status-running.png"
