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

  describe 'getClientOffer', ->
    it 'polls and promises a client offer', (done) ->
      b = new Broker 'fake'
      # fake successful request and response from broker.
      spyOn(b, '_postRequest').and.callFake ->
        b._xhr.readyState = b._xhr.DONE
        b._xhr.status = Broker.STATUS_OK
        b._xhr.responseText = 'fake offer'
        b._xhr.onreadystatechange()
      poll = b.getClientOffer()
      expect(poll).not.toBeNull()
      expect(b._postRequest).toHaveBeenCalled()
      poll.then (desc) ->
        expect(desc).toEqual 'fake offer'
        done()
      .catch ->
        fail 'should not reject on Broker.STATUS_OK'
        done()

    it 'rejects if the broker timed-out', (done) ->
      b = new Broker 'fake'
      # fake timed-out request from broker
      spyOn(b, '_postRequest').and.callFake ->
        b._xhr.readyState = b._xhr.DONE
        b._xhr.status = Broker.STATUS_GATEWAY_TIMEOUT
        b._xhr.onreadystatechange()
      poll = b.getClientOffer()
      expect(poll).not.toBeNull()
      expect(b._postRequest).toHaveBeenCalled()
      poll.then (desc) ->
        fail 'should not fulfill on GATEWAY_TIMEOUT'
        done()
      , (err) ->
        expect(err).toBe Broker.MESSAGE_TIMEOUT
        done()

    it 'rejects on any other status', (done) ->
      b = new Broker 'fake'
      # fake timed-out request from broker
      spyOn(b, '_postRequest').and.callFake ->
        b._xhr.readyState = b._xhr.DONE
        b._xhr.status = 1337
        b._xhr.onreadystatechange()
      poll = b.getClientOffer()
      expect(poll).not.toBeNull()
      expect(b._postRequest).toHaveBeenCalled()
      poll.then (desc) ->
        fail 'should not fulfill on non-OK status'
        done()
      , (err) ->
        expect(err).toBe Broker.MESSAGE_UNEXPECTED
        expect(b._xhr.status).toBe 1337
        done()

  it 'responds to the broker with answer', ->
    b = new Broker 'fake'
    spyOn(b, '_postRequest')
    b.sendAnswer 'fake id', 123
    expect(b._postRequest).toHaveBeenCalledWith(
      'fake id', jasmine.any(Object), 'answer', '123')

  it 'POST XMLHttpRequests to the broker', ->
    b = new Broker 'fake'
    b._xhr = new XMLHttpRequest()
    spyOn(b._xhr, 'open')
    spyOn(b._xhr, 'setRequestHeader')
    spyOn(b._xhr, 'send')
    b._postRequest 0, b._xhr, 'test', 'data'
    expect(b._xhr.open).toHaveBeenCalled()
    expect(b._xhr.setRequestHeader).toHaveBeenCalled()
    expect(b._xhr.send).toHaveBeenCalled()
