/* global Util, chrome, Config, UI, Broker, Snowflake, WS */
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
    setInterval((() => {
      this.stats.unshift(0);
      this.stats.splice(24);
      this.postActive();
    }), 60 * 60 * 1000);
  }

  initToggle() {
    // First, check if we have our status stored
    (new Promise((resolve) => {
      chrome.storage.local.get(["snowflake-enabled"], resolve);
    }))
    .then((result) => {
      let enabled = this.enabled;
      if (result['snowflake-enabled'] !== void 0) {
        enabled = result['snowflake-enabled'];
      } else {
        log("Toggle state not yet saved");
      }
      // If it isn't enabled, stop
      if (!enabled) {
        this.setEnabled(enabled);
        return;
      }
      // Otherwise, do feature checks
      if (!Util.hasWebRTC()) {
        this.missingFeature = 'popupWebRTCOff';
        this.setEnabled(false);
        return;
      }
      WS.probeWebsocket(config.relayAddr)
      .then(
        () => {
          this.setEnabled(true);
        },
        () => {
          log('Could not connect to bridge.');
          this.missingFeature = 'popupBridgeUnreachable';
          this.setEnabled(false);
        }
      );
    });
  }

  postActive() {
    this.setIcon();
    if (!this.port) { return; }
    this.port.postMessage({
      active: this.active,
      total: this.stats.reduce((function(t, c) {
        return t + c;
      }), 0),
      enabled: this.enabled,
      missingFeature: this.missingFeature,
    });
  }

  onConnect(port) {
    this.port = port;
    port.onDisconnect.addListener(this.onDisconnect);
    port.onMessage.addListener(this.onMessage);
    this.postActive();
  }

  onMessage(m) {
    (new Promise((resolve) => {
      chrome.storage.local.set({ "snowflake-enabled": m.enabled }, resolve);
    }))
    .then(() => {
      log("Stored toggle state");
      this.initToggle();
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
  }

  setEnabled(enabled) {
    this.enabled = enabled;
    this.postActive();
    update();
  }

  setIcon() {
    let path = null;
    if (!this.enabled) {
      path = {
        48: "assets/toolbar-off-48.png",
        96: "assets/toolbar-off-96.png"
      };
    } else if (this.active) {
      path = {
        48: "assets/toolbar-running-48.png",
        96: "assets/toolbar-running-96.png"
      };
    } else {
      path = {
        48: "assets/toolbar-on-48.png",
        96: "assets/toolbar-on-96.png"
      };
    }
    chrome.browserAction.setIcon({
      path: path,
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

  init = function() {
    config = new Config("webext");
    ui = new WebExtUI();
    broker = new Broker(config);
    snowflake = new Snowflake(config, ui, broker);
    log('== snowflake proxy ==');
    ui.initToggle();
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

  window.onunload = function() {
    if (snowflake !== null) { snowflake.disable(); }
    return null;
  };

  window.onload = init;

}());
