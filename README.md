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

Using the server plugin requires copy and pasting between 6 terminals.
Using the browser proxy (which will soon be the only way) requires copy and
pasting between 3 terminals and a browser tab.
Once a signalling facilitator is implemented 
([issue #1](https://github.com/keroserene/snowflake/issues/1))
this will become much simpler to use.

Setting up the client is the same in both cases.
Open up three terminals for the **client:**

```
cd client/
go build
```

A: `tor -f torrc SOCKSPort auto`

B: `cat > signal`

C: `tail -F snowflake.log`

##### -- Via WebRTC Server --

To connect directly to a server plugin,
open up another three terminals for the **server:**

```
cd server/
go build
```

D: `tor -f torrc`

E: `cat > signal`

F: `tail -F snowflake.log`

- Look for the offer in terminal C; copy and paste it into terminal E.
- Copy and paste the answer in terminal F to terminal B.
- At this point the tor client should bootstrap to 100%.

##### -- Via Browser Proxy --

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
