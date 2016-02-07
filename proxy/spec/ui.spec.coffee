###
jasmine tests for Snowflake UI
###

document =
  getElementById: (id) ->

describe 'UI', ->

  it 'activates debug mode when badge does not exist', ->
    spyOn(document, 'getElementById').and.callFake (id) ->
      return null if 'badge' == id
      return {
        focus: ->
      }
    u = new UI()
    expect(u.debug).toBe true
    expect(document.getElementById.calls.count()).toEqual 5
    expect(u.$status).not.toBeNull()
    expect(u.$msglog).not.toBeNull()
    expect(u.$send).not.toBeNull()
    expect(u.$input).not.toBeNull()

  it 'is not debug mode when badge exists', ->
    spyOn(document, 'getElementById').and.callFake (id) ->
      return {} if 'badge' == id
      return null
    u = new UI()
    expect(u.debug).toBe false
    expect(document.getElementById).toHaveBeenCalled()
    expect(document.getElementById.calls.count()).toEqual 1
    expect(u.$status).toBeNull()
    expect(u.$msglog).toBeNull()
    expect(u.$send).toBeNull()
    expect(u.$input).toBeNull()

  it 'sets status message only when in debug mode', ->
    u = new UI()
    u.$status = { innerHTML: '' }
    u.debug = false
    u.setStatus('test')
    expect(u.$status.innerHTML).toEqual ''
    u.debug = true
    u.setStatus('test')
    expect(u.$status.innerHTML).toEqual 'Status: test'

  it 'sets message log css correctly for debug mode', ->
    u = new UI()
    u.debug = true
    u.$msglog = {}
    u.setActive true
    expect(u.$msglog.className).toEqual 'active'
    u.setActive false
    expect(u.$msglog.className).toEqual ''

  it 'sets badge css correctly for non-debug mode', ->
    u = new UI()
    u.debug = false
    u.$badge = {}
    u.setActive true
    expect(u.$badge.className).toEqual 'active'
    u.setActive false
    expect(u.$badge.className).toEqual ''

  it 'logs to the textarea correctly, only when debug mode', ->
    u = new UI()
    u.$msglog = { value: '', scrollTop: 0, scrollHeight: 1337 }
    u.debug = false
    u.log 'test'
    expect(u.$msglog.value).toEqual ''
    expect(u.$msglog.scrollTop).toEqual 0
    u.debug = true
    u.log 'test'
    expect(u.$msglog.value).toEqual 'test\n'
    expect(u.$msglog.scrollTop).toEqual 1337
