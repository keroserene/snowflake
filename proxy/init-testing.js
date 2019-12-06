/* global TESTING, Util, Params, Config, UI, Broker, Snowflake */

/*
UI
*/

class DebugUI extends UI {

  constructor() {
    super();
    // Setup other DOM handlers if it's debug mode.
    this.$status = document.getElementById('status');
    this.$msglog = document.getElementById('msglog');
    this.$msglog.value = '';
  }

  // Status bar
  setStatus(msg) {
    var txt;
    txt = document.createTextNode('Status: ' + msg);
    while (this.$status.firstChild) {
      this.$status.removeChild(this.$status.firstChild);
    }
    return this.$status.appendChild(txt);
  }

  setActive(connected) {
    super.setActive(connected);
    return this.$msglog.className = connected ? 'active' : '';
  }

  log(msg) {
    // Scroll to latest
    this.$msglog.value += msg + '\n';
    return this.$msglog.scrollTop = this.$msglog.scrollHeight;
  }

}

// DOM elements references.
DebugUI.prototype.$msglog = null;

DebugUI.prototype.$status = null;

/*
Entry point.
*/

var snowflake, query, debug, ui, silenceNotifications, log, dbg, init;

(function() {

  if (((typeof TESTING === "undefined" || TESTING === null) || !TESTING) && !Util.featureDetect()) {
    console.log('webrtc feature not detected. shutting down');
    return;
  }

  snowflake = null;

  query = new URLSearchParams(location.search);

  debug = Params.getBool(query, 'debug', false);

  silenceNotifications = Params.getBool(query, 'silent', false);

  // Log to both console and UI if applicable.
  // Requires that the snowflake and UI objects are hooked up in order to
  // log to console.
  log = function(msg) {
    console.log('Snowflake: ' + msg);
    return snowflake != null ? snowflake.ui.log(msg) : void 0;
  };

  dbg = function(msg) {
    if (debug || ((snowflake != null ? snowflake.ui : void 0) instanceof DebugUI)) {
      return log(msg);
    }
  };

  init = function() {
    var broker, config, ui;
    config = new Config("testing");
    if ('off' !== query['ratelimit']) {
      config.rateLimitBytes = Params.getByteCount(query, 'ratelimit', config.rateLimitBytes);
    }
    ui = null;
    if (document.getElementById('status') !== null) {
      ui = new DebugUI();
    } else {
      ui = new UI();
    }
    broker = new Broker(config);
    snowflake = new Snowflake(config, ui, broker);
    log('== snowflake proxy ==');
    if (Util.snowflakeIsDisabled(config.cookieName)) {
      // Do not activate the proxy if any number of conditions are true.
      log('Currently not active.');
      return;
    }
    // Otherwise, begin setting up WebRTC and acting as a proxy.
    dbg('Contacting Broker at ' + broker.url);
    snowflake.setRelayAddr(config.relayAddr);
    return snowflake.beginWebRTC();
  };

  // Notification of closing tab with active proxy.
  window.onbeforeunload = function() {
    if (
      !silenceNotifications &&
      snowflake !== null &&
      ui.active
    ) {
      return Snowflake.MESSAGE.CONFIRMATION;
    }
    return null;
  };

  window.onunload = function() {
    if (snowflake !== null) { snowflake.disable(); }
    return null;
  };

  window.onload = init;

}());
