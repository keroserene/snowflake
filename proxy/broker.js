/* global log, dbg, snowflake */

/*
Communication with the snowflake broker.

Browser snowflakes must register with the broker in order
to get assigned to clients.
*/

// Represents a broker running remotely.
class Broker {

  // When interacting with the Broker, snowflake must generate a unique session
  // ID so the Broker can keep track of each proxy's signalling channels.
  // On construction, this Broker object does not do anything until
  // |getClientOffer| is called.
  constructor(url) {
    this.getClientOffer = this.getClientOffer.bind(this);
    this._postRequest = this._postRequest.bind(this);

    this.url = url;
    this.clients = 0;
    if (0 === this.url.indexOf('localhost', 0)) {
      // Ensure url has the right protocol + trailing slash.
      this.url = 'http://' + this.url;
    }
    if (0 !== this.url.indexOf('http', 0)) {
      this.url = 'https://' + this.url;
    }
    if ('/' !== this.url.substr(-1)) {
      this.url += '/';
    }
  }

  // Promises some client SDP Offer.
  // Registers this Snowflake with the broker using an HTTP POST request, and
  // waits for a response containing some client offer that the Broker chooses
  // for this proxy..
  // TODO: Actually support multiple clients.
  getClientOffer(id) {
    return new Promise((fulfill, reject) => {
      var xhr;
      xhr = new XMLHttpRequest();
      xhr.onreadystatechange = function() {
        if (xhr.DONE !== xhr.readyState) {
          return;
        }
        switch (xhr.status) {
          case Broker.STATUS.OK:
            return fulfill(xhr.responseText); // Should contain offer.
          case Broker.STATUS.GATEWAY_TIMEOUT:
            return reject(Broker.MESSAGE.TIMEOUT);
          default:
            log('Broker ERROR: Unexpected ' + xhr.status + ' - ' + xhr.statusText);
            snowflake.ui.setStatus(' failure. Please refresh.');
            return reject(Broker.MESSAGE.UNEXPECTED);
        }
      };
      this._xhr = xhr; // Used by spec to fake async Broker interaction
      return this._postRequest(id, xhr, 'proxy', id);
    });
  }

  // Assumes getClientOffer happened, and a WebRTC SDP answer has been generated.
  // Sends it back to the broker, which passes it to back to the original client.
  sendAnswer(id, answer) {
    var xhr;
    dbg(id + ' - Sending answer back to broker...\n');
    dbg(answer.sdp);
    xhr = new XMLHttpRequest();
    xhr.onreadystatechange = function() {
      if (xhr.DONE !== xhr.readyState) {
        return;
      }
      switch (xhr.status) {
        case Broker.STATUS.OK:
          dbg('Broker: Successfully replied with answer.');
          return dbg(xhr.responseText);
        case Broker.STATUS.GONE:
          return dbg('Broker: No longer valid to reply with answer.');
        default:
          dbg('Broker ERROR: Unexpected ' + xhr.status + ' - ' + xhr.statusText);
          return snowflake.ui.setStatus(' failure. Please refresh.');
      }
    };
    return this._postRequest(id, xhr, 'answer', JSON.stringify(answer));
  }

  // urlSuffix for the broker is different depending on what action
  // is desired.
  _postRequest(id, xhr, urlSuffix, payload) {
    var err;
    try {
      xhr.open('POST', this.url + urlSuffix);
      xhr.setRequestHeader('X-Session-ID', id);
    } catch (error) {
      err = error;
      /*
      An exception happens here when, for example, NoScript allows the domain
      on which the proxy badge runs, but not the domain to which it's trying
      to make the HTTP xhr. The exception message is like "Component
      returned failure code: 0x805e0006 [nsIXMLHttpRequest.open]" on Firefox.
      */
      log('Broker: exception while connecting: ' + err.message);
      return;
    }
    return xhr.send(payload);
  }

}

Broker.STATUS = {
  OK: 200,
  GONE: 410,
  GATEWAY_TIMEOUT: 504
};

Broker.MESSAGE = {
  TIMEOUT: 'Timed out waiting for a client offer.',
  UNEXPECTED: 'Unexpected status.'
};

Broker.prototype.clients = 0;
