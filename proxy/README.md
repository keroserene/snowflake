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

### Preparing to deploy

Background information:
 * https://bugs.torproject.org/23947#comment:8
 * https://help.torproject.org/tsa/doc/static-sites/
 * https://help.torproject.org/tsa/doc/ssh-jump-host/

You need to be in LDAP group "snowflake" and have set up an SSH key with your LDAP account.
In your ~/.ssh/config file, you should have something like:

```
Host staticiforme
HostName staticiforme.torproject.org
User <your user name>
ProxyJump people.torproject.org
IdentityFile ~/.ssh/tor
```

### Deploying

```
npm run build
```

Do a "dry run" rsync with `-n` to check that only expected files are being changed. If you don't understand why a file would be updated, you can add the `-i` option to see the reason.

```
rsync -n --delete -crv build/ staticiforme:/srv/snowflake.torproject.org/htdocs/
```

If it looks good, then repeat the rsync without `-n`.

```
rsync --delete -crv build/ staticiforme:/srv/snowflake.torproject.org/htdocs/
```

Then run the command to copy the new files to the live web servers:

```
ssh staticiforme 'static-update-component snowflake.torproject.org'
```

### Parameters

With no parameters,
snowflake uses the default relay `snowflake.freehaven.net:443` and
uses automatic signaling with the default broker at
`https://snowflake-broker.freehaven.net/`.
