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

The Snowflake client can be configured with either command line options or SOCKS options. We have a few example `torrc` files in this directory. We recommend the following `torrc` options by default:
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

### Registration methods

The Snowflake client supports a few different ways of communicating with the broker.
This initial step is sometimes called rendezvous.

#### Domain fronting HTTPS

For domain fronting rendezvous, use the `-url` and `-front` command-line options together.
[Domain fronting](https://www.bamsoftware.com/papers/fronting/)
hides the externally visible domain name from an external observer,
making it appear that the Snowflake client is communicating with some server
other than the Snowflake broker.

* `-url` is the HTTPS URL of a forwarder to the broker, on some service that supports domain fronting, such as a CDN.
* `-front` is the domain name to show externally. It must be another domain on the same service.

Example:
```
-url https://snowflake-broker.torproject.net.global.prod.fastly.net/ \
-front cdn.sstatic.net \
```

#### AMP cache

For AMP cache rendezvous, use the `-url`, `-ampcache`, and `-front` command-line options together.
[AMP](https://amp.dev/documentation/) is a standard for web pages for mobile computers.
An [AMP cache](https://amp.dev/documentation/guides-and-tutorials/learn/amp-caches-and-cors/how_amp_pages_are_cached/)
is a cache and proxy specialized for AMP pages.
The Snowflake broker has the ability to make its client registration responses look like AMP pages,
so it can be accessed through an AMP cache.
When you use AMP cache rendezvous, it appears to an observer that the Snowflake client
is accessing an AMP cache, or some other domain operated by the same organization.
You still need to use the `-front` command-line option, because the
[format of AMP cache URLs](https://amp.dev/documentation/guides-and-tutorials/learn/amp-caches-and-cors/amp-cache-urls/)
would otherwise reveal the domain name of the broker.

There is only one AMP cache that works with this option,
the Google AMP cache at https://cdn.ampproject.org/.

* `-url` is the HTTPS URL of the broker.
* `-ampcache` is `https://cdn.ampproject.org/`.
* `-front` is any Google domain, such as `www.google.com`.

Example:
```
-url https://snowflake-broker.torproject.net/ \
-ampcache https://cdn.ampproject.org/ \
-front www.google.com \
```

#### Direct access

It is also possible to access the broker directly using HTTPS, without domain fronting,
for testing purposes. This mode is not suitable for circumvention, because the
broker is easily blocked by its address.
