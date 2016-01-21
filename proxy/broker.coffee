###
Communication with the snowflake broker.

Browser snowflakes must register with the broker in order
to get assigned to clients.
###

# Represents a broker running remotely.
class Broker
  # When interacting with the Broker, snowflake must generate a unique session
  # ID so the Broker can keep track of which signalling channel it's speaking
  # to.
  constructor: (@url) ->
    log 'Using Broker at ' + @url

  # Snowflake registers with the broker using an HTTP POST request, and expects
  # a response from the broker containing some client offer
  register: ->
    # base_url = this.fac_url.replace(/\?.*/, "");
    # url = base_url + "?" + build_query_string(params);
    xhr = new XMLHttpRequest()
    try
      xhr.open 'POST', @url
      xhr
    catch err
      ###
      An exception happens here when, for example, NoScript allows the domain on
      which the proxy badge runs, but not the domain to which it's trying to
      make the HTTP request. The exception message is like "Component returned
      failure code: 0x805e0006 [nsIXMLHttpRequest.open]" on Firefox.
      ###
      log 'Broker: exception while connecting: ' + err.message
      return

    # xhr.responseType = 'text'
    xhr.onreadystatechange = ->
      if xhr.DONE == xhr.readyState
        if 200 == xhr.status
          log 'Broker: success'
          log 'Response: ' + xhr.responseText
          # @fac_complete xhr.responseText
        else
          log 'Broker error ' + xhr.status + ' - ' + xhr.statusText

    xhr.send 'snowflake-testing'

  sendAnswer: (answer) ->
    log 'Sending answer to broker.'
    log answer
