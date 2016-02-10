###
jasmine tests for Snowflake broker
###

# fake xhr
# class XMLHttpRequest
class XMLHttpRequest
  constructor: ->
    @onreadystatechange = null
  open: ->
  setRequestHeader: ->
  send: ->
  DONE: 1

describe 'Broker', ->

  it 'can be created', ->
    b = new Broker 'fake'
    expect(b.url).toEqual 'https://fake/'
    expect(b.id).not.toBeNull()

  it 'polls and promises a client offer', (done) ->
    b = new Broker 'fake'
    # fake successful request
    spyOn(b, 'sendRequest').and.callFake ->
      b.request.readyState = b.request.DONE
      b.request.status = STATUS_OK
      b.request.responseText = 'test'
      b.request.onreadystatechange()
    poll = b.getClientOffer()
    expect(poll).not.toBeNull()
    poll.then (desc) =>
      expect(desc).toEqual 'test'
      done()

  it 'requests correctly', ->
    b = new Broker 'fake'
    b.request = new XMLHttpRequest()
    spyOn(b.request, 'open')
    spyOn(b.request, 'setRequestHeader')
    spyOn(b.request, 'send')
    b.sendRequest()
    expect(b.request.open).toHaveBeenCalled()
    expect(b.request.setRequestHeader).toHaveBeenCalled()
    expect(b.request.send).toHaveBeenCalled()

  it 'responds to the broker with answer', ->
    # TODO: fix
    b = new Broker 'fake'
    b.sendAnswer 'foo'
