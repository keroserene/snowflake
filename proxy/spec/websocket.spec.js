/* global expect, it, describe, WS */

/*
jasmine tests for Snowflake websocket
*/

describe('BuildUrl', function() {

  it('should parse just protocol and host', function() {
    expect(WS.buildUrl('http', 'example.com')).toBe('http://example.com');
  });

  it('should handle different ports', function() {
    expect(WS.buildUrl('http', 'example.com', 80)).toBe('http://example.com');
    expect(WS.buildUrl('http', 'example.com', 81)).toBe('http://example.com:81');
    expect(WS.buildUrl('http', 'example.com', 443)).toBe('http://example.com:443');
    expect(WS.buildUrl('http', 'example.com', 444)).toBe('http://example.com:444');
  });

  it('should handle paths', function() {
    expect(WS.buildUrl('http', 'example.com', 80, '/')).toBe('http://example.com/');
    expect(WS.buildUrl('http', 'example.com', 80, '/test?k=%#v')).toBe('http://example.com/test%3Fk%3D%25%23v');
    expect(WS.buildUrl('http', 'example.com', 80, '/test')).toBe('http://example.com/test');
  });

  it('should handle params', function() {
    expect(WS.buildUrl('http', 'example.com', 80, '/test', [['k', '%#v']])).toBe('http://example.com/test?k=%25%23v');
    expect(WS.buildUrl('http', 'example.com', 80, '/test', [['a', 'b'], ['c', 'd']])).toBe('http://example.com/test?a=b&c=d');
  });

  it('should handle ips', function() {
    expect(WS.buildUrl('http', '1.2.3.4')).toBe('http://1.2.3.4');
    expect(WS.buildUrl('http', '1:2::3:4')).toBe('http://[1:2::3:4]');
  });

  it('should handle bogus', function() {
    expect(WS.buildUrl('http', 'bog][us')).toBe('http://bog%5D%5Bus');
    expect(WS.buildUrl('http', 'bog:u]s')).toBe('http://bog%3Au%5Ds');
  });

});
