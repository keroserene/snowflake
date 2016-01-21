# snowflake-pt

A Pluggable Transport using WebRTC

### Status

- Successful automatic bootstraps with a WebRTC transport,
  using HTTP signaling (with optional domain fronting) speaking to
  a multitude of volunteer "snowflakes".
- Needs a lot more work though.

### Usage



```
cd client/
go build
tor -f torrc
```

And it will start the client plugin with the following `torrc`
options:
```
ClientTransportPlugin snowflake exec ./client \
--url https://snowflake-reg.appspot.com/ \
--front www.google.com
```

It will speak to the Broker, get matched with a "snowflake" browser proxy,
and negotiate a WebRTC PeerConnection.
After that, it should bootstrap to 100%.

To see logs, do `tail -F snowflake.log` in a second terminal.

You can modify the `torrc` to use your own broker,
or remove the options entirely which will default to the old copy paste
method (see `torrc-manual`):

```
ClientTransportPlugin snowflake exec ./client --meek
```

Also, it is possible to connect directly to the go-webrtc server plugin
(skipping all the browser snowflake / broker stuff - see appendix)

### Building a Snowflake Proxy

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

Open a browser tab to `0.0.0.0:8000/snowflake.html`.

TODO: Turn the snowflake proxy into a more deployable badge.

### Appendix

##### -- Testing directly via WebRTC Server --

Using the server plugin uses an HTTP server that simulates the interaction
that a client would have with a broker.
Using the browser proxy (which will soon be the only way) requires copy and
pasting between 3 terminals and a browser tab.

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

##### -- Via Browser Proxy --

Open up three terminals for the **client:**

A: `tor -f torrc SOCKSPort auto`

B: `cat > signal`

C: `tail -F snowflake.log`

Then, in the browser proxy:

- Look for the offer in terminal C; copy and paste it into the browser.
- Copy and paste the answer generated in the browser back to terminal B.
- Once WebRTC successfully connects, the browser terminal should turn green.
  Shortly after, the tor client should bootstrap to 100%.

More documentation on the way.
