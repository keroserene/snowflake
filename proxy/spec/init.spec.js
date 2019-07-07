/* global expect, it, describe, Snowflake, UI */

// Fake snowflake to interact with

var snowflake = {
  ui: new UI,
  broker: {
    sendAnswer: function() {}
  },
  state: Snowflake.MODE.INIT
};

describe('Init', function() {

  it('gives a dialog when closing, only while active', function() {
    silenceNotifications = false;
    snowflake.state = Snowflake.MODE.WEBRTC_READY;
    var msg = window.onbeforeunload();
    expect(snowflake.state).toBe(Snowflake.MODE.WEBRTC_READY);
    expect(msg).toBe(Snowflake.MESSAGE.CONFIRMATION);
    snowflake.state = Snowflake.MODE.INIT;
    msg = window.onbeforeunload();
    expect(snowflake.state).toBe(Snowflake.MODE.INIT);
    expect(msg).toBe(null);
  });

  it('does not give a dialog when silent flag is on', function() {
    silenceNotifications = true;
    snowflake.state = Snowflake.MODE.WEBRTC_READY;
    var msg = window.onbeforeunload();
    expect(snowflake.state).toBe(Snowflake.MODE.WEBRTC_READY);
    expect(msg).toBe(null);
  });

});
