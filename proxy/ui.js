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
