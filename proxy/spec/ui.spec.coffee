###
jasmine tests for Snowflake UI
###

document =
  getElementById: (id) -> {}
  createTextNode: (txt) -> txt

describe 'UI', ->

  it 'activates debug mode when badge does not exist', ->
    spyOn(document, 'getElementById').and.callFake (id) ->
      return null if 'badge' == id
      return {}
    u = new DebugUI()
    expect(document.getElementById.calls.count()).toEqual 2
    expect(u.$status).not.toBeNull()
    expect(u.$msglog).not.toBeNull()

  it 'is not debug mode when badge exists', ->
    spyOn(document, 'getElementById').and.callFake (id) ->
      return {} if 'badge' == id
      return null
    u = new BadgeUI()
    expect(document.getElementById).toHaveBeenCalled()
    expect(document.getElementById.calls.count()).toEqual 1
    expect(u.$badge).not.toBeNull()

  it 'sets status message when in debug mode', ->
    u = new DebugUI()
    u.$status =
      innerHTML: ''
      appendChild: (txt) -> @innerHTML = txt
    u.setStatus('test')
    expect(u.$status.innerHTML).toEqual 'Status: test'

  it 'sets message log css correctly for debug mode', ->
    u = new DebugUI()
    u.setActive true
    expect(u.$msglog.className).toEqual 'active'
    u.setActive false
    expect(u.$msglog.className).toEqual ''

  it 'sets badge css correctly for non-debug mode', ->
    u = new BadgeUI()
    u.$badge = {}
    u.setActive true
    expect(u.$badge.className).toEqual 'active'
    u.setActive false
    expect(u.$badge.className).toEqual ''

  it 'logs to the textarea correctly when debug mode', ->
    u = new DebugUI()
    u.$msglog = { value: '', scrollTop: 0, scrollHeight: 1337 }
    u.log 'test'
    expect(u.$msglog.value).toEqual 'test\n'
    expect(u.$msglog.scrollTop).toEqual 1337
