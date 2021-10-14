<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**

- [Dependencies](#dependencies)
- [Building the standalone Snowflake proxy](#building-the-standalone-snowflake-proxy)
- [Running a standalone Snowflake proxy](#running-a-standalone-snowflake-proxy)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

This is a standalone (not browser-based) version of the Snowflake proxy. For browser-based versions of the Snowflake proxy, see https://gitlab.torproject.org/tpo/anti-censorship/pluggable-transports/snowflake-webext.

### Dependencies

- Go 1.13+
- We use the [pion/webrtc](https://github.com/pion/webrtc) library for WebRTC communication with Snowflake proxies. Note: running `go get` will fetch this dependency automatically during the build process.

### Building the standalone Snowflake proxy

To build the Snowflake proxy, make sure you are in the `proxy/` directory, and then run:

```
go get
go build
```

### Running a standalone Snowflake proxy

The Snowflake proxy can be run with the following options:
```
Usage of ./proxy:
  -broker string
        broker URL (default "https://snowflake-broker.torproject.net/")
  -capacity uint
        maximum concurrent clients
  -keep-local-addresses
        keep local LAN address ICE candidates
  -log string
        log filename
  -relay string
        websocket relay URL (default "wss://snowflake.torproject.net/")
  -stun string
        stun URL (default "stun:stun.stunprotocol.org:3478")
  -unsafe-logging
        prevent logs from being scrubbed
```

For more information on how to run a Snowflake proxy in deployment, see our [community documentation](https://community.torproject.org/relay/setup/snowflake/standalone/).
