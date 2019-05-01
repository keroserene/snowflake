This is the Tor client component of Snowflake.

It is based on goptlib.

### Flags

The client uses these following `torrc` options by default:
```
ClientTransportPlugin snowflake exec ./client \
-url https://snowflake-broker.azureedge.net/ \
-front ajax.aspnetcdn.com \
-ice stun:stun.l.google.com:19302
```

`-url` should be the URL of a Broker instance.

`-front` is an optional front domain for the Broker request.

`-ice` is a comma-separated list of ICE servers. These can be STUN or TURN
servers.
