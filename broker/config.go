/*
This is the server-side code that runs on Google App Engine for the
"appspot" registration method.

See doc/appspot-howto.txt for more details about setting up an
application, and advice on running one.

To upload a new version:
$ torify ~/go_appengine/appcfg.py --no_cookies -A $YOUR_APP_ID update .
*/
package snowflake_broker

// host:port/basepath of the broker you want to register with
// for example, fp-broker.org or example.com:12345/broker
// https:// and /reg/ will be prepended and appended respectively.
const SNOWFLAKE_BROKER = ""
