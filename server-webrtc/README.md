Ordinarily, the WebRTC client plugin speaks with a Broker which helps
match and signal with a browser proxy, which ultimately speaks with a default
websocket server.


However, this directory contains a WebRTC server plugin which uses an
HTTP server that simulates the interaction that a client would have with
the broker, for direct testing.

Edit server-webrtc/torrc and add "-http 127.0.0.1:8080" to the end of the
ServerTransportPlugin line:
```
ServerTransportPlugin snowflake exec ./server-webrtc -http 127.0.0.1:8080
```

```
cd server-webrtc/
go build
tor -f torrc
```

Edit client/torrc and add "-url http://127.0.0.1:8080" to the end of the
ClientTransportPlugin line:
```
ClientTransportPlugin snowflake exec ./client -url http://127.0.0.1:8080/
```
