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

Setting up the client is the same in both cases.
Open up three terminals for the **client:**

```
cd client/
go build
```

A: tor -f torrc SOCKSPort auto

B: cat > signal

C: tail -F snowflake.log

Now, to connect directly to a server plugin:

Open up another three terminals for the **server:**

```
cd server/
go build
```

D: tor -f torrc

E: cat > signal

F: tail -F snowflake.log

Look for the offer in terminal C; copy and paste it into terminal E.
Copy and paste the answer in terminal F to terminal B.
At this point the tor client should bootstrap to 100%.

#### Snowflake proxy

Otherwise, to connect through the WebRTC proxy in the browser, start a local
http server in the `proxy/` directory however you wish. For instance:
```
cd proxy/
python -m http.server
```
Open a browser tab to `0.0.0.0:8000/snowflake.html`.
The page will ask you to input a relay.
Input your desired relay address, or input nothing/gibberish which will cause
snowflake to use a default relay.

Look for the offer in terminal C; copy and paste it into the browser.
Copy and paste the answer generated in the browser back to terminal B.
At this point the tor client should bootstrap to 100%.


### More

More documentation on the way.
