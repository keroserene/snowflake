class Config
  brokerUrl: 'snowflake-broker.bamsoftware.com'
  relayAddr:
    host: 'snowflake.bamsoftware.com'
    port: '443'
    # Original non-wss relay:
    # host: '192.81.135.242'
    # port: 9902

  cookieName: "snowflake-allow"

  # Bytes per second. Set to undefined to disable limit.
  rateLimitBytes: undefined
  minRateLimit: 10 * 1024
  rateLimitHistory: 5.0
  defaultBrokerPollInterval: 5.0 * 1000

  maxNumClients: 1
  connectionsPerClient: 1

  # TODO: Different ICE servers.
  pcConfig = {
    iceServers: [
      { urls: ['stun:stun.l.google.com:19302'] }
    ]
  }
