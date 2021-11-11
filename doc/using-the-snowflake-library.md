Snowflake is available as a general-purpose pluggable transports library and adheres to the [pluggable transports v2.1 Go API](https://github.com/Pluggable-Transports/Pluggable-Transports-spec/blob/master/releases/PTSpecV2.1/Pluggable%20Transport%20Specification%20v2.1%20-%20Go%20Transport%20API.pdf).

### Client library

The Snowflake client library contains functions for running a Snowflake client.

Example usage:

```Golang
package main

import (
    "log"

    sf "git.torproject.org/pluggable-transports/snowflake.git/v2/client/lib"
)

func main() {

    config := sf.ClientConfig{
        BrokerURL:   "https://snowflake-broker.example.com",
        FrontDomain: "https://friendlyfrontdomain.net",
        ICEAddresses: []string{
            "stun:stun.voip.blackberry.com:3478",
            "stun:stun.stunprotocol.org:3478"},
        Max: 1,
    }
    transport, err := sf.NewSnowflakeClient(config)
    if err != nil {
        log.Fatal("Failed to start snowflake transport: ", err)
    }

    // transport implements the ClientFactory interface and returns a net.Conn
    conn, err := transport.Dial()
    if err != nil {
        log.Printf("dial error: %s", err)
        return
    }
    defer conn.Close()

    // ...

}
```

#### Using your own rendezvous method

You can define and use your own rendezvous method to communicate with a Snowflake broker by implementing the `RendezvousMethod` interface.

```Golang

package main

import (
    "log"

    sf "git.torproject.org/pluggable-transports/snowflake.git/v2/client/lib"
)

type StubMethod struct {
}

func (m *StubMethod) Exchange(pollReq []byte) ([]byte, error) {
    var brokerResponse []byte
    var err error

    //Implement the logic you need to communicate with the Snowflake broker here

    return brokerResponse, err
}

func main() {
    config := sf.ClientConfig{
        ICEAddresses:       []string{
            "stun:stun.voip.blackberry.com:3478",
            "stun:stun.stunprotocol.org:3478"},
    }
    transport, err := sf.NewSnowflakeClient(config)
    if err != nil {
        log.Fatal("Failed to start snowflake transport: ", err)
    }

    // custom rendezvous methods can be set with `SetRendezvousMethod`
    rendezvous := &StubMethod{}
    transport.SetRendezvousMethod(rendezvous)

    // transport implements the ClientFactory interface and returns a net.Conn
    conn, err := transport.Dial()
    if err != nil {
        log.Printf("dial error: %s", err)
        return
    }
    defer conn.Close()

    // ...

}
```

### Server library

The Snowflake server library contains functions for running a Snowflake server.

Example usage:
```Golang

package main

import (
    "log"
    "net"

    sf "git.torproject.org/pluggable-transports/snowflake.git/v2/server/lib"
    "golang.org/x/crypto/acme/autocert"
)

func main() {

    // The snowflake server runs a websocket server. To run this securely, you will
    // need a valid certificate.
    certManager := &autocert.Manager{
        Prompt:     autocert.AcceptTOS,
        HostPolicy: autocert.HostWhitelist("snowflake.yourdomain.com"),
        Email:      "you@yourdomain.com",
    }

    transport := sf.NewSnowflakeServer(certManager.GetCertificate)

    addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:443")
    if err != nil {
        log.Printf("error resolving bind address: %s", err.Error())
    }
    ln, err := transport.Listen(addr)
    if err != nil {
        log.Printf("error opening listener: %s", err.Error())
    }
    for {
        conn, err := ln.Accept()
        if err != nil {
            if err, ok := err.(net.Error); ok && err.Temporary() {
                continue
            }
            log.Printf("Snowflake accept error: %s", err)
            break
        }
        go func() {
            // ...

            defer conn.Close()
        }()
    }

    // ...

}

```
### Running your own Snowflake infrastructure

At the moment we do not have the ability to share Snowfake infrastructure between different types of applications. If you are planning on using Snowflake as a transport for your application, you will need to:

- Run a Snowflake broker. See our [broker documentation](../broker/) and [installation guide](https://gitlab.torproject.org/tpo/anti-censorship/team/-/wikis/Survival-Guides/Snowflake-Broker-Installation-Guide) for more information

- Run Snowflake proxies. These can be run as [standalone Go proxies](../proxy/) or [browser-based proxies](https://gitlab.torproject.org/tpo/anti-censorship/pluggable-transports/snowflake-webext).
