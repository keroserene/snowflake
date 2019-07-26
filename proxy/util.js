/* exported Util, Params, DummyRateLimit */

/*
A JavaScript WebRTC snowflake proxy

Contains helpers for parsing query strings and other utilities.
*/

class Util {

  static genSnowflakeID() {
    return Math.random().toString(36).substring(2);
  }

  static hasWebRTC() {
    return typeof PeerConnection === 'function';
  }

  static hasCookies() {
    return navigator.cookieEnabled;
  }

}


class Parse {

  // Parse a cookie data string (usually document.cookie). The return type is an
  // object mapping cookies names to values. Returns null on error.
  // http://www.w3.org/TR/DOM-Level-2-HTML/html.html#ID-8747038
  static cookie(cookies) {
    var i, j, len, name, result, string, strings, value;
    result = {};
    strings = [];
    if (cookies) {
      strings = cookies.split(';');
    }
    for (i = 0, len = strings.length; i < len; i++) {
      string = strings[i];
      j = string.indexOf('=');
      if (-1 === j) {
        return null;
      }
      name = decodeURIComponent(string.substr(0, j).trim());
      value = decodeURIComponent(string.substr(j + 1).trim());
      if (!(name in result)) {
        result[name] = value;
      }
    }
    return result;
  }

  // Parse an address in the form 'host:port'. Returns an Object with keys 'host'
  // (String) and 'port' (int). Returns null on error.
  static address(spec) {
    var host, m, port;
    m = null;
    if (!m) {
      // IPv6 syntax.
      m = spec.match(/^\[([\0-9a-fA-F:.]+)\]:([0-9]+)$/);
    }
    if (!m) {
      // IPv4 syntax.
      m = spec.match(/^([0-9.]+):([0-9]+)$/);
    }
    if (!m) {
      // TODO: Domain match
      return null;
    }
    host = m[1];
    port = parseInt(m[2], 10);
    if (isNaN(port) || port < 0 || port > 65535) {
      return null;
    }
    return {
      host: host,
      port: port
    };
  }

  // Parse a count of bytes. A suffix of 'k', 'm', or 'g' (or uppercase)
  // does what you would think. Returns null on error.
  static byteCount(spec) {
    let matches = spec.match(/^(\d+(?:\.\d*)?)(\w*)$/);
    if (matches === null) {
      return null;
    }
    let count = Number(matches[1]);
    if (isNaN(count)) {
      return null;
    }
    const UNITS = new Map([
      ['', 1],
      ['k', 1024],
      ['m', 1024*1024],
      ['g', 1024*1024*1024],
    ]);
    let unit = matches[2].toLowerCase();
    if (!UNITS.has(unit)) {
      return null;
    }
    let multiplier = UNITS.get(unit);
    return count * multiplier;
  }

  // Parse a connection-address out of the "c=" Connection Data field of a
  // session description. Return undefined if none is found.
  // https://tools.ietf.org/html/rfc4566#section-5.7
  static ipFromSDP(sdp) {
    var i, len, m, pattern, ref;
    ref = [/^c=IN IP4 ([\d.]+)(?:(?:\/\d+)?\/\d+)?(:? |$)/m, /^c=IN IP6 ([0-9A-Fa-f:.]+)(?:\/\d+)?(:? |$)/m];
    for (i = 0, len = ref.length; i < len; i++) {
      pattern = ref[i];
      m = pattern.exec(sdp);
      if (m != null) {
        return m[1];
      }
    }
  }

}


class Params {

  static getBool(query, param, defaultValue) {
    if (!query.has(param)) {
      return defaultValue;
    }
    var val;
    val = query.get(param);
    if ('true' === val || '1' === val || '' === val) {
      return true;
    }
    if ('false' === val || '0' === val) {
      return false;
    }
    return null;
  }

  // Get an object value and parse it as a byte count. Example byte counts are
  // '100' and '1.3m'. Returns |defaultValue| if param is not a key. Return null
  // on a parsing error.
  static getByteCount(query, param, defaultValue) {
    if (!query.has(param)) {
      return defaultValue;
    }
    return Parse.byteCount(query.get(param));
  }

}


class BucketRateLimit {

  constructor(capacity, time) {
    this.capacity = capacity;
    this.time = time;
  }

  age() {
    var delta, now;
    now = new Date();
    delta = (now - this.lastUpdate) / 1000.0;
    this.lastUpdate = now;
    this.amount -= delta * this.capacity / this.time;
    if (this.amount < 0.0) {
      return this.amount = 0.0;
    }
  }

  update(n) {
    this.age();
    this.amount += n;
    return this.amount <= this.capacity;
  }

  // How many seconds in the future will the limit expire?
  when() {
    this.age();
    return (this.amount - this.capacity) / (this.capacity / this.time);
  }

  isLimited() {
    this.age();
    return this.amount > this.capacity;
  }

}

BucketRateLimit.prototype.amount = 0.0;

BucketRateLimit.prototype.lastUpdate = new Date();


// A rate limiter that never limits.
class DummyRateLimit {

  constructor(capacity, time) {
    this.capacity = capacity;
    this.time = time;
  }

  update() {
    return true;
  }

  when() {
    return 0.0;
  }

  isLimited() {
    return false;
  }

}
