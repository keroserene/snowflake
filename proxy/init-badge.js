/* global Util, Params, Config, UI, Broker, Snowflake, Popup, Parse, availableLangs, WS */

/*
UI
*/

class Messages {
  constructor(json) {
    this.json = json;
  }
  getMessage(m, ...rest) {
    let message = this.json[m].message;
    return message.replace(/\$(\d+)/g, (...args) => {
      return rest[Number(args[1]) - 1];
    });
  }
}

let messages = null;

class BadgeUI extends UI {

  constructor() {
    super();
    this.popup = new Popup();
  }

  setStatus() {}

  missingFeature(missing) {
    this.popup.setEnabled(false);
    this.popup.setActive(false);
    this.popup.setStatusText(messages.getMessage('popupStatusOff'));
    this.setIcon('off');
    this.popup.setStatusDesc(missing, true);
    this.popup.hideButton();
  }

  turnOn() {
    const clients = this.active ? 1 : 0;
    this.popup.setChecked(true);
    if (clients > 0) {
      this.popup.setStatusText(messages.getMessage('popupStatusOn', String(clients)));
      this.setIcon('running');
    } else {
      this.popup.setStatusText(messages.getMessage('popupStatusReady'));
      this.setIcon('on');
    }
    // FIXME: Share stats from webext
    this.popup.setStatusDesc('');
    this.popup.setEnabled(true);
    this.popup.setActive(this.active);
  }

  turnOff() {
    this.popup.setChecked(false);
    this.popup.setStatusText(messages.getMessage('popupStatusOff'));
    this.setIcon('off');
    this.popup.setStatusDesc('');
    this.popup.setEnabled(false);
    this.popup.setActive(false);
  }

  setActive(connected) {
    super.setActive(connected);
    this.turnOn();
  }

  setIcon(status) {
    document.getElementById('icon').href = `assets/toolbar-${status}.ico`;
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

const defaultLang = 'en_US';

// Resolve as in,
// https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Internationalization#Localized_string_selection
function getLang() {
  let lang = navigator.language || defaultLang;
  lang = lang.replace(/-/g, '_');
  if (availableLangs.has(lang)) {
    return lang;
  }
  lang = lang.split('_')[0];
  if (availableLangs.has(lang)) {
    return lang;
  }
  return defaultLang;
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
    if (cookies[COOKIE_NAME] !== '1') {
      ui.turnOff();
      snowflake.disable();
      log('Currently not active.');
      return;
    }

    if (!Util.hasWebRTC()) {
      ui.missingFeature(messages.getMessage('popupWebRTCOff'));
      snowflake.disable();
      return;
    }

    WS.probeWebsocket(config.relayAddr)
    .then(
      () => {
        ui.turnOn();
        dbg('Contacting Broker at ' + broker.url);
        log('Starting snowflake');
        snowflake.setRelayAddr(config.relayAddr);
        snowflake.beginWebRTC();
      },
      () => {
        ui.missingFeature(messages.getMessage('popupBridgeUnreachable'));
        snowflake.disable();
        log('Could not connect to bridge.');
      }
    );
  };

  init = function() {
    ui = new BadgeUI();

    if (!Util.hasCookies()) {
      ui.missingFeature(messages.getMessage('badgeCookiesOff'));
      return;
    }

    config = new Config("badge");
    if ('off' !== query.get('ratelimit')) {
      config.rateLimitBytes = Params.getByteCount(query, 'ratelimit', config.rateLimitBytes);
    }
    broker = new Broker(config);
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

  window.onload = function() {
    fetch(`./_locales/${getLang()}/messages.json`)
    .then((res) => {
      if (!res.ok) { return; }
      return res.json();
    })
    .then((json) => {
      messages = new Messages(json);
      Popup.fill(document.body, (m) => {
        return messages.getMessage(m);
      });
      init();
    });
  }

}());
