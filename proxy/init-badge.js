/*
Entry point.
*/

if (((typeof TESTING === "undefined" || TESTING === null) || !TESTING) && !Util.featureDetect()) {
  console.log('webrtc feature not detected. shutting down');
  return;
}

var snowflake = null;

var query = Query.parse(location);

var debug = Params.getBool(query, 'debug', false);

var silenceNotifications = Params.getBool(query, 'silent', false);

// Log to both console and UI if applicable.
// Requires that the snowflake and UI objects are hooked up in order to
// log to console.
var log = function(msg) {
  console.log('Snowflake: ' + msg);
  return snowflake != null ? snowflake.ui.log(msg) : void 0;
};

var dbg = function(msg) {
  if (debug || ((snowflake != null ? snowflake.ui : void 0) instanceof DebugUI)) {
    return log(msg);
  }
};

var init = function() {
  var broker, config, ui;
  config = new Config;
  if ('off' !== query['ratelimit']) {
    config.rateLimitBytes = Params.getByteCount(query, 'ratelimit', config.rateLimitBytes);
  }
  ui = null;
  if (document.getElementById('badge') !== null) {
    ui = new BadgeUI();
  } else if (document.getElementById('status') !== null) {
    ui = new DebugUI();
  } else {
    ui = new UI();
  }
  broker = new Broker(config.brokerUrl);
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
