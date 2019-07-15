/* global expect, it, describe, spyOn, DebugUI */
/* eslint no-redeclare: 0 */

/*
jasmine tests for Snowflake UI
*/

var document = {
  getElementById: function() {
    return {};
  },
  createTextNode: function(txt) {
    return txt;
  }
};

describe('UI', function() {

  it('activates debug mode when badge does not exist', function() {
    var u;
    spyOn(document, 'getElementById').and.callFake(function(id) {
      if ('badge' === id) {
        return null;
      }
      return {};
    });
    u = new DebugUI();
    expect(document.getElementById.calls.count()).toEqual(2);
    expect(u.$status).not.toBeNull();
    expect(u.$msglog).not.toBeNull();
  });

  it('sets status message when in debug mode', function() {
    var u;
    u = new DebugUI();
    u.$status = {
      innerHTML: '',
      appendChild: function(txt) {
        return this.innerHTML = txt;
      }
    };
    u.setStatus('test');
    expect(u.$status.innerHTML).toEqual('Status: test');
  });

  it('sets message log css correctly for debug mode', function() {
    var u;
    u = new DebugUI();
    u.setActive(true);
    expect(u.$msglog.className).toEqual('active');
    u.setActive(false);
    expect(u.$msglog.className).toEqual('');
  });

  it('logs to the textarea correctly when debug mode', function() {
    var u;
    u = new DebugUI();
    u.$msglog = {
      value: '',
      scrollTop: 0,
      scrollHeight: 1337
    };
    u.log('test');
    expect(u.$msglog.value).toEqual('test\n');
    expect(u.$msglog.scrollTop).toEqual(1337);
  });

});
