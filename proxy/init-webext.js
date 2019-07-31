/* global Util, chrome, Config, UI, Broker, Snowflake */
/* eslint no-unused-vars: 0 */

/*
UI
*/

class WebExtUI extends UI {

  constructor() {
    super();
    this.onConnect = this.onConnect.bind(this);
    this.onMessage = this.onMessage.bind(this);
    this.onDisconnect = this.onDisconnect.bind(this);
    this.initStats();
    chrome.runtime.onConnect.addListener(this.onConnect);
  }

  initStats() {
    this.stats = [0];
    return setInterval((() => {
      this.stats.unshift(0);
      this.stats.splice(24);
      return this.postActive();
    }), 60 * 60 * 1000);
  }

  initToggle() {
    chrome.storage.local.get("snowflake-enabled", (result) => {
      if (result['snowflake-enabled'] !== void 0) {
        this.enabled = result['snowflake-enabled'];
      } else {
        log("Toggle state not yet saved");
      }
      this.setEnabled(this.enabled);
    });
  }

  postActive() {
    var ref;
    return (ref = this.port) != null ? ref.postMessage({
      active: this.active,
      total: this.stats.reduce((function(t, c) {
        return t + c;
      }), 0),
      enabled: this.enabled
    }) : void 0;
  }

  onConnect(port) {
    this.port = port;
    port.onDisconnect.addListener(this.onDisconnect);
    port.onMessage.addListener(this.onMessage);
    return this.postActive();
  }

  onMessage(m) {
    this.enabled = m.enabled;
    this.setEnabled(this.enabled);
    this.postActive();
    chrome.storage.local.set({
      "snowflake-enabled": this.enabled
    }, function() {
      log("Stored toggle state");
    });
  }

  onDisconnect() {
    this.port = null;
  }

  setActive(connected) {
    super.setActive(connected);
    if (connected) {
      this.stats[0] += 1;
    }
    this.postActive();
    if (this.active) {
      return chrome.browserAction.setIcon({
        path: {
          32: "assets/status-running.png"
        }
      });
    } else {
      return chrome.browserAction.setIcon({
        path: {
          32: "assets/status-on.png"
        }
      });
    }
  }

  setEnabled(enabled) {
    update();
    return chrome.browserAction.setIcon({
      path: {
        32: "assets/status-" + (enabled ? "on" : "off") + ".png"
      }
    });
  }

}

WebExtUI.prototype.port = null;

WebExtUI.prototype.stats = null;

WebExtUI.prototype.enabled = true;

/*
Entry point.
*/

var debug, snowflake, config, broker, ui, log, dbg, init, update, silenceNotifications;

(function () {

  silenceNotifications = false;
  debug = false;
  snowflake = null;
  config = null;
  broker = null;
  ui = null;

  // Log to both console and UI if applicable.
  // Requires that the snowflake and UI objects are hooked up in order to
  // log to console.
  log = function(msg) {
    console.log('Snowflake: ' + msg);
    return snowflake != null ? snowflake.ui.log(msg) : void 0;
  };

  dbg = function(msg) {
    if (debug) {
      return log(msg);
    }
  };

  if (!Util.hasWebRTC()) {
    chrome.runtime.onConnect.addListener(function(port) {
      return port.postMessage({
        missingFeature: true
      });
    });
    chrome.browserAction.setIcon({ path: { 32: "assets/status-off.png" } });
    return;
  }

  init = function() {
    config = new Config;
    ui = new WebExtUI();
    broker = new Broker(config.brokerUrl);
    snowflake = new Snowflake(config, ui, broker);
    log('== snowflake proxy ==');
    return ui.initToggle();
  };

  update = function() {
    if (!ui.enabled) {
      // Do not activate the proxy if any number of conditions are true.
      snowflake.disable();
      log('Currently not active.');
      return;
    }
    // Otherwise, begin setting up WebRTC and acting as a proxy.
    dbg('Contacting Broker at ' + broker.url);
    log('Starting snowflake');
    snowflake.setRelayAddr(config.relayAddr);
    return snowflake.beginWebRTC();
  };

  // Notification of closing tab with active proxy.
  window.onbeforeunload = function() {
    if (
      !silenceNotifications &&
      snowflake !== null &&
      Snowflake.MODE.WEBRTC_READY === snowflake.state
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
