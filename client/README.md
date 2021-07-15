<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**

- [Dependencies](#dependencies)
- [Building the Snowflake client](#building-the-snowflake-client)
- [Running the Snowflake client with Tor](#running-the-snowflake-client-with-tor)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

This is the Tor client component of Snowflake.

It is based on the [goptlib](https://gitweb.torproject.org/pluggable-transports/goptlib.git/) pluggable transports library for Tor.


### Dependencies

- Go 1.13+
- We use the [pion/webrtc](https://github.com/pion/webrtc) library for WebRTC communication with Snowflake proxies. Note: running `go get` will fetch this dependency automatically during the build process.

### Building the Snowflake client

To build the Snowflake client, make sure you are in the `client/` directory, and then run:

```
go get
go build
```

### Running the Snowflake client with Tor

We have an example `torrc` file in this repository. The client uses these following `torrc` options by default:
```
UseBridges 1

ClientTransportPlugin snowflake exec ./client \
-url https://snowflake-broker.torproject.net.global.prod.fastly.net/ \
-front cdn.sstatic.net \
-ice stun:stun.voip.blackberry.com:3478,stun:stun.altar.com.pl:3478,stun:stun.antisip.com:3478,stun:stun.bluesip.net:3478,stun:stun.dus.net:3478,stun:stun.epygi.com:3478,stun:stun.sonetel.com:3478,stun:stun.sonetel.net:3478,stun:stun.stunprotocol.org:3478,stun:stun.uls.co.za:3478,stun:stun.voipgate.com:3478,stun:stun.voys.nl:3478

Bridge snowflake 192.0.2.3:1
```

`-url` is the URL of a broker instance. If you would like to try out Snowflake with your own broker, simply provide the URL of your broker instance with this option.

`-front` is an optional front domain for the broker request.

`-ice` is a comma-separated list of ICE servers. These can be STUN or TURN servers. We recommend using servers that have implemented NAT discovery. See our wiki page on [NAT traversal](https://gitlab.torproject.org/tpo/anti-censorship/pluggable-transports/snowflake/-/wikis/NAT-matching) for more information.

To bootstrap Tor, run:
```
tor -f torrc
```
This should start the client plugin, bootstrapping to 100% using WebRTC.
