###
Communication with the snowflake broker.

Browser snowflakes must register with the broker in order
to get assigned to clients.
###

STATUS_OK = 200
STATUS_GONE = 410
STATUS_GATEWAY_TIMEOUT = 504

genSnowflakeID = ->
  Math.random().toString(36).substring(2)

# Represents a broker running remotely.
class Broker

  clients: 0
  id: null

  # When interacting with the Broker, snowflake must generate a unique session
  # ID so the Broker can keep track of which signalling channel it's speaking
  # to.
  constructor: (@url) ->
    @clients = 0
    @id = genSnowflakeID()
    log 'Contacting Broker at ' + @url + '\nSnowflake ID: ' + @id

  # Snowflake registers with the broker using an HTTP POST request, and expects
  # a response from the broker containing some client offer.
  # TODO: Actually support multiple clients.
  getClientOffer: ->
    new Promise (fulfill, reject) =>
      xhr = new XMLHttpRequest()
      try
        xhr.open 'POST', @url + 'proxy'
        xhr.setRequestHeader('X-Session-ID', @id)
      catch err
        ###
        An exception happens here when, for example, NoScript allows the domain
        on which the proxy badge runs, but not the domain to which it's trying
        to make the HTTP request. The exception message is like "Component
        returned failure code: 0x805e0006 [nsIXMLHttpRequest.open]" on Firefox.
        ###
        log 'Broker: exception while connecting: ' + err.message
        return
      xhr.onreadystatechange = ->
        return if xhr.DONE != xhr.readyState
        switch xhr.status
          when STATUS_OK
            fulfill xhr.responseText  # Should contain offer.
          when STATUS_GATEWAY_TIMEOUT
            reject 'Timed out waiting for a client to serve. Retrying...'
          else
            log 'Broker ERROR: Unexpected ' + xhr.status +
                ' - ' + xhr.statusText
      xhr.send @id
      log @id + " - polling for client offer..."

  sendAnswer: (answer) ->
    log @id + ' - Sending answer back to broker...\n'
    log answer.sdp
    xhr = new XMLHttpRequest()
    try
      xhr.open 'POST', @url + 'answer'
      xhr.setRequestHeader('X-Session-ID', @id)
    catch err
      log 'Broker: exception while connecting: ' + err.message
      return
    xhr.onreadystatechange = ->
      return if xhr.DONE != xhr.readyState
      switch xhr.status
        when STATUS_OK
          log 'Broker: Successfully replied with answer.'
          log xhr.responseText
        when STATUS_GONE
          log 'Broker: No longer valid to reply with answer.'
        else
          log 'Broker ERROR: Unexpected ' + xhr.status +
              ' - ' + xhr.statusText
    xhr.send JSON.stringify(answer)
