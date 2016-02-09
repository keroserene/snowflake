###
jasmine tests for Snowflake broker
###

# fake xhr
class XMLHttpRequest
  open: ->
  send: ->
  onreadystatechange: ->
  DONE: 1

describe 'Broker', ->

  it 'can be created', ->
    b = new Broker('fake')
    expect(b.url).toEqual 'https://fake/'
    expect(b.id).not.toBeNull()

  it 'polls for client offer', (done) ->
    b = new Broker('fake')
    # TODO: fix this
    poll = b.getClientOffer()
    spyOn(b.request, 'open')
    spyOn(b.request, 'send').and.callFake ->
      b.onreadystatechange()
    poll.then = ->
      done()
    expect(poll).not.toBeNull()
    # expect(b.request.open).toHaveBeenCalled()
    # expect(b.request.send).toHaveBeenCalled()
    # fake successful poll
    b.request.readyState = XMLHttpRequest.DONE
    b.request.status = STATUS_OK
    b.request.responseText = 'test'
    done()

  it 'responds to the broker with answer', ->
    # TODO
