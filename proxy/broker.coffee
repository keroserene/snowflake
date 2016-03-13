###
Communication with the snowflake broker.

Browser snowflakes must register with the broker in order
to get assigned to clients.
###

STATUS_OK = 200
STATUS_GONE = 410
STATUS_GATEWAY_TIMEOUT = 504

MESSAGE_TIMEOUT = 'Timed out waiting for a client offer.'
MESSAGE_UNEXPECTED = 'Unexpected status.'

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
    # Ensure url has the right protocol + trailing slash.
    @url = 'http://' + @url if 0 == @url.indexOf('localhost', 0)
    @url = 'https://' + @url if 0 != @url.indexOf('http', 0)
    @url += '/' if '/' != @url.substr -1

  # Promises some client SDP Offer.
  # Registers this Snowflake with the broker using an HTTP POST request, and
  # waits for a response containing some client offer that the Broker chooses
  # for this proxy..
  # TODO: Actually support multiple clients.
  getClientOffer: =>
    new Promise (fulfill, reject) =>
      xhr = new XMLHttpRequest()
      xhr.onreadystatechange = ->
        return if xhr.DONE != xhr.readyState
        switch xhr.status
          when STATUS_OK
            fulfill xhr.responseText  # Should contain offer.
          when STATUS_GATEWAY_TIMEOUT
            reject MESSAGE_TIMEOUT
          else
            log 'Broker ERROR: Unexpected ' + xhr.status +
                ' - ' + xhr.statusText
            snowflake.ui.setStatus ' failure. Please refresh.'
            reject MESSAGE_UNEXPECTED
      @_xhr = xhr  # Used by spec to fake async Broker interaction
      @_postRequest xhr, 'proxy', @id

  # Assumes getClientOffer happened, and a WebRTC SDP answer has been generated.
  # Sends it back to the broker, which passes it to back to the original client.
  sendAnswer: (answer) ->
    dbg @id + ' - Sending answer back to broker...\n'
    dbg answer.sdp
    xhr = new XMLHttpRequest()
    xhr.onreadystatechange = ->
      return if xhr.DONE != xhr.readyState
      switch xhr.status
        when STATUS_OK
          dbg 'Broker: Successfully replied with answer.'
          dbg xhr.responseText
        when STATUS_GONE
          dbg 'Broker: No longer valid to reply with answer.'
        else
          dbg 'Broker ERROR: Unexpected ' + xhr.status +
              ' - ' + xhr.statusText
          snowflake.ui.setStatus ' failure. Please refresh.'
    @_postRequest xhr, 'answer', JSON.stringify(answer)

  # urlSuffix for the broker is different depending on what action
  # is desired.
  _postRequest: (xhr, urlSuffix, payload) =>
    try
      xhr.open 'POST', @url + urlSuffix
      xhr.setRequestHeader('X-Session-ID', @id)
    catch err
      ###
      An exception happens here when, for example, NoScript allows the domain
      on which the proxy badge runs, but not the domain to which it's trying
      to make the HTTP xhr. The exception message is like "Component
      returned failure code: 0x805e0006 [nsIXMLHttpRequest.open]" on Firefox.
      ###
      log 'Broker: exception while connecting: ' + err.message
      return
    xhr.send payload
