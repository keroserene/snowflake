/*
Only websocket-specific stuff.
*/

class WS {

  // Build an escaped URL string from unescaped components. Only scheme and host
  // are required. See RFC 3986, section 3.
  static buildUrl(scheme, host, port, path, params) {
    var parts;
    parts = [];
    parts.push(encodeURIComponent(scheme));
    parts.push('://');
    // If it contains a colon but no square brackets, treat it as IPv6.
    if (host.match(/:/) && !host.match(/[[\]]/)) {
      parts.push('[');
      parts.push(host);
      parts.push(']');
    } else {
      parts.push(encodeURIComponent(host));
    }
    if (void 0 !== port && this.DEFAULT_PORTS[scheme] !== port) {
      parts.push(':');
      parts.push(encodeURIComponent(port.toString()));
    }
    if (void 0 !== path && '' !== path) {
      if (!path.match(/^\//)) {
        path = '/' + path;
      }
      path = path.replace(/[^/]+/, function(m) {
        return encodeURIComponent(m);
      });
      parts.push(path);
    }
    if (void 0 !== params) {
      parts.push('?');
      parts.push(new URLSearchParams(params).toString());
    }
    return parts.join('');
  }

  static makeWebsocket(addr, params) {
    var url, ws, wsProtocol;
    wsProtocol = this.WSS_ENABLED ? 'wss' : 'ws';
    url = this.buildUrl(wsProtocol, addr.host, addr.port, '/', params);
    ws = new WebSocket(url);
    /*
    'User agents can use this as a hint for how to handle incoming binary data:
    if the attribute is set to 'blob', it is safe to spool it to disk, and if it
    is set to 'arraybuffer', it is likely more efficient to keep the data in
    memory.'
    */
    ws.binaryType = 'arraybuffer';
    return ws;
  }

  static probeWebsocket(addr) {
    return new Promise((resolve, reject) => {
      const ws = WS.makeWebsocket(addr);
      ws.onopen = () => {
        resolve();
        ws.close();
      };
      ws.onerror = () => {
        reject();
        ws.close();
      };
    });
  }

}

WS.WSS_ENABLED = true;

WS.DEFAULT_PORTS = {
  http: 80,
  https: 443
};
