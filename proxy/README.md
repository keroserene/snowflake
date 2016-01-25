This is the browser proxy component of Snowflake.

### Testing:

Unit tests are available with:
```
cake test
```

To run locally, start a webserver and navigate to `snowflake.html`.

### Parameters

With no parameters,
snowflake uses the default relay `192.81.135.242:9901` and
uses automatic signaling with the default broker at
`https://snowflake-reg.appspot.com/`.


Here are optional parameters to include in the query string.
```
manual - enables copy-paste signalling mode.
relay=<address> - use a custom target relay.
broker=<url> - use a custom broker.
```
