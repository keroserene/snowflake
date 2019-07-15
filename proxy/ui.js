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
