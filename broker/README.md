<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**

- [Overview](#overview)
- [Running your own](#running-your-own)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

This is the Broker component of Snowflake.

### Overview

The Broker handles the rendezvous by matching Snowflake
Clients with Proxies, and passing their WebRTC Session Descriptions
(the "signaling" step). This allows Clients and Proxies to establish
a Peer connection.

It is analogous to Flashproxy's
[Facilitator](https://trac.torproject.org/projects/tor/wiki/FlashProxyFAQ),
but bidirectional and domain-fronted.

The Broker expects:

- Clients to send their SDP offer in a POST request, which will then block
  until the Broker responds with the answer of the matched Proxy.
- Proxies to announce themselves with a POST request, to which the Broker
  responds with some Client's SDP offer. The Proxy should then send a second
  POST request soon after containing its SDP answer, which the Broker passes
  back to the same Client.

### Running your own

The server uses TLS by default.
There is a `--disable-tls` option for testing purposes,
but you should use TLS in production.

The server automatically fetches certificates
from [Let's Encrypt](https://en.wikipedia.org/wiki/Let's_Encrypt) as needed.
Use the `--acme-hostnames` option to tell the server
what hostnames it may request certificates for.
You can optionally provide a contact email address,
using the `--acme-email` option,
so that Let's Encrypt can inform you of any problems.

In order to fetch certificates automatically,
the server needs to open an additional HTTP listener on port 80.
On Linux, you can use the `setcap` program,
part of libcap2, to enable the broker to bind to low-numbered ports
without having to run as root:
```
setcap 'cap_net_bind_service=+ep' /usr/local/bin/broker
```
You can control the listening broker port with the --addr option.
Port 443 is the default.

You'll need to provide the URL of the custom broker
to the client plugin using the `--url $URL` flag.
