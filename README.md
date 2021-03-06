# Snowflake

[![Build Status](https://travis-ci.org/keroserene/snowflake.svg?branch=master)](https://travis-ci.org/keroserene/snowflake)

Pluggable Transport using WebRTC, inspired by Flashproxy.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**

- [Usage](#usage)
  - [Dependencies](#dependencies)
  - [More Info](#more-info)
  - [Building](#building)
  - [Test Environment](#test-environment)
- [FAQ](#faq)
- [Appendix](#appendix)
    - [-- Testing with Standalone Proxy --](#---testing-with-standalone-proxy---)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

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
- [pion/webrtc](https://github.com/pion/webrtc)
- Go 1.13+

---

#### More Info

Tor can plug in the Snowflake client via a correctly configured `torrc`.
For example:

```
ClientTransportPlugin snowflake exec ./client \
-url https://snowflake-broker.azureedge.net/ \
-front ajax.aspnetcdn.com \
-ice stun:stun.l.google.com:19302
-max 3
```

The flags `-url` and `-front` allow the Snowflake client to speak to the Broker,
in order to get connected with some volunteer's browser proxy. `-ice` is a
comma-separated list of ICE servers, which are required for NAT traversal.

For logging, run `tail -F snowflake.log` in a second terminal.

You can modify the `torrc` to use your own broker:

```
ClientTransportPlugin snowflake exec ./client --meek
```


#### Test Environment

There is a Docker-based test environment at https://github.com/cohosh/snowbox.


### FAQ

**Q: How does it work?**

In the Tor use-case:

1. Volunteers visit websites which host the "snowflake" proxy. (just
like flashproxy)
2. Tor clients automatically find available browser proxies via the Broker
(the domain fronted signaling channel).
3. Tor client and browser proxy establish a WebRTC peer connection.
4. Proxy connects to some relay.
5. Tor occurs.

More detailed information about how clients, snowflake proxies, and the Broker
fit together on the way...

**Q: What are the benefits of this PT compared with other PTs?**

Snowflake combines the advantages of flashproxy and meek. Primarily:

- It has the convenience of Meek, but can support magnitudes more
users with negligible CDN costs. (Domain fronting is only used for brief
signalling / NAT-piercing to setup the P2P WebRTC DataChannels which handle
the actual traffic.)

- Arbitrarily high numbers of volunteer proxies are possible like in
flashproxy, but NATs are no longer a usability barrier - no need for
manual port forwarding!

**Q: Why is this called Snowflake?**

It utilizes the "ICE" negotiation via WebRTC, and also involves a great
abundance of ephemeral and short-lived (and special!) volunteer proxies...

### Appendix

##### -- Testing with Standalone Proxy --

```
cd proxy
go build
./proxy
```

More documentation on the way.

Also available at:
[torproject.org/pluggable-transports/snowflake](https://gitweb.torproject.org/pluggable-transports/snowflake.git/)
