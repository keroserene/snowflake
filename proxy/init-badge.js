/* global Util, Params, Config, UI, Broker, Snowflake, Popup, Parse */

/*
UI
*/

class BadgeUI extends UI {

  constructor() {
    super();
    this.popup = new Popup();
  }

  setStatus() {}

  missingFeature(missing) {
    this.popup.setImgSrc('off');
    this.popup.setStatusText("Snowflake is off");
    this.popup.setStatusDesc(missing, true);
    this.popup.hideButton();
  }

  turnOn() {
    const clients = this.active ? 1 : 0;
    this.popup.setChecked(true);
    this.popup.setToggleText('Turn Off');
    this.popup.setStatusText(`${clients} client${(clients !== 1) ? 's' : ''} connected.`);
    // FIXME: Share stats from webext
    const total = 0;
    this.popup.setStatusDesc(`Your snowflake has helped ${total} user${(total !== 1) ? 's' : ''} circumvent censorship in the last 24 hours.`);
    this.popup.setImgSrc(this.active ? "running" : "on");
  }

  turnOff() {
    this.popup.setChecked(false);
    this.popup.setToggleText('Turn On');
    this.popup.setStatusText("Snowflake is off");
    this.popup.setStatusDesc("");
    this.popup.setImgSrc("off");
  }

  setActive(connected) {
    super.setActive(connected);
    this.turnOn();
  }

}

BadgeUI.prototype.popup = null;


/*
Entry point.
*/

// Defaults to opt-in.
var COOKIE_NAME = "snowflake-allow";
var COOKIE_LIFETIME = "Thu, 01 Jan 2038 00:00:00 GMT";
var COOKIE_EXPIRE = "Thu, 01 Jan 1970 00:00:01 GMT";

function setSnowflakeCookie(val, expires) {
  document.cookie = `${COOKIE_NAME}=${val}; path=/; expires=${expires};`;
}

var debug, snowflake, config, broker, ui, log, dbg, init, update, silenceNotifications, query;

(function() {

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
    if (debug) { log(msg); }
  };

  update = function() {
    const cookies = Parse.cookie(document.cookie);
    if (cookies[COOKIE_NAME] === '1') {
      ui.turnOn();
      dbg('Contacting Broker at ' + broker.url);
      log('Starting snowflake');
      snowflake.setRelayAddr(config.relayAddr);
      snowflake.beginWebRTC();
    } else {
      ui.turnOff();
      snowflake.disable();
      log('Currently not active.');
    }
  };

  init = function() {
    ui = new BadgeUI();

    if (!Util.hasWebRTC()) {
      ui.missingFeature("WebRTC feature is not detected.");
      return;
    }

    if (!Util.hasCookies()) {
      ui.missingFeature("Cookies are not enabled.");
      return;
    }

    config = new Config;
    if ('off' !== query.get('ratelimit')) {
      config.rateLimitBytes = Params.getByteCount(query, 'ratelimit', config.rateLimitBytes);
    }
    broker = new Broker(config.brokerUrl);
    snowflake = new Snowflake(config, ui, broker);
    log('== snowflake proxy ==');
    update();

    document.getElementById('enabled').addEventListener('change', (event) => {
      if (event.target.checked) {
        setSnowflakeCookie('1', COOKIE_LIFETIME);
      } else {
        setSnowflakeCookie('', COOKIE_EXPIRE);
      }
      update();
    })
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
