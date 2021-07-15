<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**

- [Overview](#overview)
- [Running your own](#running-your-own)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

This is code for a remote probe test component of Snowflake.

### Overview

This is a probe test server to allow proxies to test their compatability
with Snowflake. Right now the only type of test implemented is a
compatability check for clients with symmetric NATs.

### Running your own

The server uses TLS by default.
There is a `--disable-tls` option for testing purposes,
but you should use TLS in production.

To build the probe server, run
```go build```

To deploy the probe server, first set the necessary env variables with
```
export HOSTNAMES=${YOUR HOSTNAMES}
export EMAIL=${YOUR EMAIL}
```
then run ```docker-compose up```

Setting up a symmetric NAT configuration requires a few extra steps. After
upping the docker container, run
```docker inspect snowflake-probetest```
to find the subnet used by the probetest container. Then run
```sudo iptables -L -t nat``` to find the POSTROUTING rules for the subnet.
It should look something like this:
```
Chain POSTROUTING (policy ACCEPT)
target     prot opt source               destination
MASQUERADE  all  --  172.19.0.0/16        anywhere
```
to modify this rule, execute the command
```sudo iptables -t nat -R POSTROUTING $RULE_NUM -s 172.19.0.0/16 -j MASQUERADE --random```
where RULE_NUM is the numbered rule corresponding to your docker container's subnet masquerade rule.
Afterwards, you should see the rule changed to be:
```
Chain POSTROUTING (policy ACCEPT)
target     prot opt source               destination
MASQUERADE  all  --  172.19.0.0/16        anywhere      random
```
