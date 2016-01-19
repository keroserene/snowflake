# snowflake-pt

A Pluggable Transport using WebRTC

### Status

- Successfully bootstraps over WebRTC, both directly to a server plugin,
  as well as through the browser which proxies WebRTC to websocket.
- Needs work on signaling with the facilitator.

### Usage

There are currently two ways to try this:
- Directly to the go-webrtc server plugin.
- Through a browser snowflake proxy.

Using the server plugin uses an HTTP server that simulates the interaction
that a client would have with a facilitator.
Using the browser proxy (which will soon be the only way) requires copy and
pasting between 3 terminals and a browser tab.
Once a signalling facilitator is implemented 
([issue #1](https://github.com/keroserene/snowflake/issues/1))
this will become much simpler to use.

##### -- Via WebRTC Server --

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

```
cd client/
go build
tor -f torrc
```

At this point the tor client should bootstrap to 100%.

##### -- Via Browser Proxy --

Open up three terminals for the **client:**

A: `tor -f torrc SOCKSPort auto`

B: `cat > signal`

C: `tail -F snowflake.log`


To connect through the WebRTC browser proxy, first make sure
coffeescript is installed. Then, build with:
```
cd proxy/
cake build
```

Then start a local http server in the `proxy/build/` in any way you like.
For instance:

```
cd build/
python -m http.server
```

Open a browser tab to `0.0.0.0:8000/snowflake.html`.
Input your desired relay address, or nothing/gibberish, which will cause
snowflake to just use a default relay.

- Look for the offer in terminal C; copy and paste it into the browser.
- Copy and paste the answer generated in the browser back to terminal B.
- Once WebRTC successfully connects, the browser terminal should turn green.
  Shortly after, the tor client should bootstrap to 100%.


### More

More documentation on the way.
