/*
jasmine tests for Snowflake UI
*/
var document = {
  getElementById: function(id) {
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

  it('is not debug mode when badge exists', function() {
    var u;
    spyOn(document, 'getElementById').and.callFake(function(id) {
      if ('badge' === id) {
        return {};
      }
      return null;
    });
    u = new BadgeUI();
    expect(document.getElementById).toHaveBeenCalled();
    expect(document.getElementById.calls.count()).toEqual(1);
    expect(u.$badge).not.toBeNull();
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

  it('sets badge css correctly for non-debug mode', function() {
    var u;
    u = new BadgeUI();
    u.$badge = {};
    u.setActive(true);
    expect(u.$badge.className).toEqual('active');
    u.setActive(false);
    expect(u.$badge.className).toEqual('');
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
