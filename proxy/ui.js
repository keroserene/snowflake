/* global chrome, log, update */

/*
All of Snowflake's DOM manipulation and inputs.
*/

class UI {

  setStatus() {}

  setActive(connected) {
    return this.active = connected;
  }

  log() {}

}

UI.prototype.active = false;

UI.prototype.enabled = true;


class BadgeUI extends UI {

  constructor() {
    super();
    this.$badge = document.getElementById('badge');
  }

  setActive(connected) {
    super.setActive(connected);
    return this.$badge.className = connected ? 'active' : '';
  }

}

BadgeUI.prototype.$badge = null;


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
          32: "icons/status-running.png"
        }
      });
    } else {
      return chrome.browserAction.setIcon({
        path: {
          32: "icons/status-on.png"
        }
      });
    }
  }

  setEnabled(enabled) {
    update();
    return chrome.browserAction.setIcon({
      path: {
        32: "icons/status-" + (enabled ? "on" : "off") + ".png"
      }
    });
  }

}

WebExtUI.prototype.port = null;

WebExtUI.prototype.stats = null;
