# Snowflake

[![Build Status](https://travis-ci.org/keroserene/snowflake.svg?branch=master)](https://travis-ci.org/keroserene/snowflake)

A Pluggable Transport using WebRTC

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**

- [Status](#status)
- [Usage](#usage)
  - [Dependencies](#dependencies)
  - [More Info](#more-info)
  - [Building a Snowflake](#building-a-snowflake)
- [Appendix](#appendix)
    - [-- Testing directly via WebRTC Server --](#---testing-directly-via-webrtc-server---)
    - [-- Via Browser Proxy --](#---via-browser-proxy---)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

### Status

Successfully & automatically bootstraps with a WebRTC transport, using HTTP
signaling (with optional domain fronting) speaking to a multitude of volunteer
"snowflakes". Still lots of work to do.

### Usage

```
cd client/
go get
go build
tor -f torrc
```
This should start the client plugin, bootstrapping to 100% using WebRTC.

#### Dependencies

Client:
- [go-webrtc](https://github.com/keroserene/go-webrtc).
- Go 1.5+

Proxy:
- [CoffeeScript](coffeescript.org)

---

#### More Info

The client uses the following `torrc` options:
```
ClientTransportPlugin snowflake exec ./client \
-url https://snowflake-reg.appspot.com/ \
-front www.google.com \
-ice stun:stun.l.google.com:19302
```

Which allows it to speak to the Broker,
get matched with a "snowflake" browser proxy,
and negotiate a WebRTC PeerConnection.

To see logs, do `tail -F snowflake.log` in a second terminal.

You can modify the `torrc` to use your own broker,
or remove the options entirely which will default to the old copy paste
method (see `torrc-manual`):

```
ClientTransportPlugin snowflake exec ./client --meek
```

Also, it is possible to connect directly to the go-webrtc server plugin
(skipping all the browser snowflake / broker stuff - see appendix)


#### Building a Snowflake

This will only work if there are any browser snowflakes running at all.
To run your own, first make sure coffeescript is installed.
Then, build with:

```
cd proxy/
cake build
```
(Type `cake` by itself to see possible commands)

Then, start a local http server in the `proxy/build/` in any way you like.
For instance:

```
cd build/
python -m http.server
```

Then, open a browser tab to `0.0.0.0:8000/snowflake.html`,
which causes you to act as an ephemeral Tor bridge.

### Appendix

##### -- Testing directly via WebRTC Server --

Ordinarily, the WebrTC client plugin speaks with a Broker which helps
match and signal with a browser proxy, which ultimately speaks with a default
websocket server.


However, there is a WebRTC server plugin which uses an HTTP server that
simulates the interaction that a client would have with the broker, for
direct testing.

Edit server/torrc and add "-http 127.0.0.1:8080" to the end of the
ServerTransportPlugin line:
```
ServerTransportPlugin snowflake exec ./server -http 127.0.0.1:8080
```

```
cd server/
go build
tor -f torrc
```

Edit client/torrc and add "-url http://127.0.0.1:8080" to the end of the
ClientTransportPlugin line:
```
ClientTransportPlugin snowflake exec ./client -url http://127.0.0.1:8080/
```

##### -- Testing Copy-Paste Via Browser Proxy --

Open up three terminals for the **client:**

A: `tor -f torrc-manual SOCKSPort auto`

B: `cat > signal`

C: `tail -F snowflake.log`

Then, in the browser proxy:

- Look for the offer in terminal C; copy and paste it into the browser.
- Copy and paste the answer generated in the browser back to terminal B.
- Once WebRTC successfully connects, the browser terminal should turn green.
  Shortly after, the tor client should bootstrap to 100%.

More documentation on the way.

Also available at:
[torproject.org/pluggable-transports/snowflake](https://gitweb.torproject.org/pluggable-transports/snowflake.git/)
