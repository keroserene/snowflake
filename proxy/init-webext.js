/*
Entry point.
*/

var debug = false;

var snowflake = null;

var config = null;

var broker = null;

var ui = null;

// Log to both console and UI if applicable.
// Requires that the snowflake and UI objects are hooked up in order to
// log to console.
var log = function(msg) {
  console.log('Snowflake: ' + msg);
  return snowflake != null ? snowflake.ui.log(msg) : void 0;
};

var dbg = function(msg) {
  if (debug) {
    return log(msg);
  }
};

if (!Util.featureDetect()) {
  chrome.runtime.onConnect.addListener(function(port) {
    return port.postMessage({
      missingFeature: true
    });
  });
  return;
}

var init = function() {
  config = new Config;
  ui = new WebExtUI();
  broker = new Broker(config.brokerUrl);
  snowflake = new Snowflake(config, ui, broker);
  log('== snowflake proxy ==');
  return ui.initToggle();
};

var update = function() {
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
  if (!silenceNotifications && Snowflake.MODE.WEBRTC_READY === snowflake.state) {
    return Snowflake.MESSAGE.CONFIRMATION;
  }
  return null;
};

window.onunload = function() {
  snowflake.disable();
  return null;
};

window.onload = init;
