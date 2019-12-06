This is the browser proxy component of Snowflake.

### Embedding

See https://snowflake.torproject.org/ for more info:
```
<iframe src="https://snowflake.torproject.org/embed.html" width="88" height="16" frameborder="0" scrolling="no"></iframe>
```

### Building the badge / snowflake.torproject.org

```
npm install
npm run build
```

which outputs to the `build/` directory.

### Building the webextension

```
npm install
npm run webext
```

and then load the `webext/` directory as an unpacked extension.
 * https://developer.mozilla.org/en-US/docs/Tools/about:debugging#Loading_a_temporary_extension
 * https://developer.chrome.com/extensions/getstarted#manifest

### Testing

Unit testing with Jasmine are available with:
```
npm install
npm test
```

To run locally, start an http server in `build/` and navigate to `/embed.html`.

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
npm install
npm run build
```

Do a "dry run" rsync with `-n` to check that only expected files are being changed. If you don't understand why a file would be updated, you can add the `-i` option to see the reason.

```
rsync -n --chown=:snowflake --chmod ug=rw,D+x --perms --delete -crv build/ staticiforme:/srv/snowflake.torproject.org/htdocs/
```

If it looks good, then repeat the rsync without `-n`.

```
rsync --chown=:snowflake --chmod ug=rw,D+x --perms --delete -crv build/ staticiforme:/srv/snowflake.torproject.org/htdocs/
```

You can ignore errors of the form `rsync: failed to set permissions on "<dirname>/": Operation not permitted (1)`.

Then run the command to copy the new files to the live web servers:

```
ssh staticiforme 'static-update-component snowflake.torproject.org'
```

### Parameters

With no parameters,
snowflake uses the default relay `snowflake.freehaven.net:443` and
uses automatic signaling with the default broker at
`https://snowflake-broker.freehaven.net/`.

### Reuse as a library

The badge and the webextension make use of the same underlying library and
only differ in their UI.  That same library can be produced for use with other
interfaces, such as [Cupcake][1], by running,

```
npm install
npm run library
```

which outputs a `./snowflake-library.js`.

You'd then want to create a subclass of `UI` to perform various actions as
the state of the snowflake changes,

```
class MyUI extends UI {
    ...
}
```

See `WebExtUI` in `init-webext.js` and `BadgeUI` in `init-badge.js` for
examples.

Finally, initialize the snowflake with,

```
var log = function(msg) {
  return console.log('Snowflake: ' + msg);
};
var dbg = log;

var config = new Config("myui");  // NOTE: Set a unique proxy type for metrics
var ui = new MyUI();  // NOTE: Using the class defined above
var broker = new Broker(config.brokerUrl);

var snowflake = new Snowflake(config, ui, broker);

snowflake.setRelayAddr(config.relayAddr);
snowflake.beginWebRTC();
```

This minimal setup is pretty much what's currently in `init-node.js`.

When configuring the snowflake, set a unique `proxyType` (first argument
to `Config`) that will be used when recording metrics at the broker.  Also,
it would be helpful to get in touch with the [Anti-Censorship Team][2] at the
Tor Project to let them know about your tool.

[1]: https://chrome.google.com/webstore/detail/cupcake/dajjbehmbnbppjkcnpdkaniapgdppdnc
[2]: https://trac.torproject.org/projects/tor/wiki/org/teams/AntiCensorshipTeam
