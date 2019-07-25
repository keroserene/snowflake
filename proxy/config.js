
class Config {}

Config.prototype.brokerUrl = 'snowflake-broker.freehaven.net';

Config.prototype.relayAddr = {
  host: 'snowflake.freehaven.net',
  port: '443'
};

// Original non-wss relay:
// host: '192.81.135.242'
// port: 9902
Config.prototype.cookieName = "snowflake-allow";

// Bytes per second. Set to undefined to disable limit.
Config.prototype.rateLimitBytes = void 0;

Config.prototype.minRateLimit = 10 * 1024;

Config.prototype.rateLimitHistory = 5.0;

Config.prototype.defaultBrokerPollInterval = 5.0 * 1000;

Config.prototype.maxNumClients = 1;

Config.prototype.connectionsPerClient = 1;

// TODO: Different ICE servers.
Config.prototype.pcConfig = {
  iceServers: [
    {
      urls: ['stun:stun.l.google.com:19302']
    }
  ]
};
