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

You can run your own Broker on localhost, you'll need to pass a TLS
certificate file using `--cert` option and the corresponding private key
file using `--key` option.

You'll need to provide the URL of the custom broker
to the client plugin using the `--url $URL` flag.
