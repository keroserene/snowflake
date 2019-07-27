This is the browser proxy component of Snowflake.

### Embedding

See https://snowflake.torproject.org/ for more info:
```
<iframe src="https://snowflake.torproject.org/embed.html" width="88" height="16" frameborder="0" scrolling="no"></iframe>
```

### Building

```
npm run build
```

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
snowflake uses the default relay `snowflake.freehaven.net:443` and
uses automatic signaling with the default broker at
`https://snowflake-broker.freehaven.net/`.
