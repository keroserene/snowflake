This is the browser proxy component of Snowflake.

### Embedding

See [keroserene.net/snowflake](https://keroserene.net/snowflake) for more info:
```
<iframe src="https://keroserene.net/snowflake/embed.html" width="88" height="16" frameborder="0" scrolling="no"></iframe>
```

### Building

```
cake build
```
(Type `cake` by itself to see possible commands)

### Testing

Unit testing with Jasmine are available with:
```
npm install
npm test
```

To run locally, either:
- Navigate to `proxy/build/embed.html`
- For a more fully featured "debug" version,
  start a webserver and navigate to `snowflake.html`.

### Parameters

With no parameters,
snowflake uses the default relay `snowflake.bamsoftware.com:443` and
uses automatic signaling with the default broker at
`https://snowflake-reg.appspot.com/`.

Here are optional parameters to include in the query string.
```
manual - enables copy-paste signalling mode.
relay=<address> - use a custom target relay.
broker=<url> - use a custom broker.
```
