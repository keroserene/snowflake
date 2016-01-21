###
Communication with the snowflake broker.

Browser snowflakes must register with the broker in order
to get assigned to clients.
###

STATUS_OK = 200
STATUS_GATEWAY_TIMEOUT = 504

# Represents a broker running remotely.
class Broker

  clients: 0

  # When interacting with the Broker, snowflake must generate a unique session
  # ID so the Broker can keep track of which signalling channel it's speaking
  # to.
  constructor: (@url) ->
    log 'Using Broker at ' + @url
    clients = 0

  # Snowflake registers with the broker using an HTTP POST request, and expects
  # a response from the broker containing some client offer.
  # TODO: Actually support multiple clients.
  getClientOffer: ->
    new Promise (fulfill, reject) =>
      xhr = new XMLHttpRequest()
      try
        xhr.open 'POST', @url
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

      xhr.send 'snowflake-testing'
      log "Broker: polling for client offer..."

  sendAnswer: (answer) ->
    log 'Sending answer to broker.'
    log answer
