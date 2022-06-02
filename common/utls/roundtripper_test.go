package utls

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"math/rand"
	"net/http"
	"testing"
	"time"

	stdcontext "context"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRoundTripper(t *testing.T) {
	var selfSignedCert []byte
	var selfSignedPrivateKey *rsa.PrivateKey
	httpServerContext, cancel := stdcontext.WithCancel(stdcontext.Background())
	Convey("[Test]Set up http servers", t, func(c C) {
		c.Convey("[Test]Generate Self-Signed Cert", func(c C) {
			// Ported from https://gist.github.com/samuel/8b500ddd3f6118d052b5e6bc16bc4c09

			// note that we use the insecure math/rand here because some platforms
			// fail the test suite at build time in Debian, due to entropy starvation.
			// since that's not a problem at test time, we do *not* use a secure
			// mechanism for key generation.
			//
			// DO NOT REUSE THIS CODE IN PRODUCTION, IT IS DANGEROUS
			insecureRandReader := rand.New(rand.NewSource(1337))
			priv, err := rsa.GenerateKey(insecureRandReader, 4096)
			c.So(err, ShouldBeNil)
			template := x509.Certificate{
				SerialNumber: big.NewInt(1),
				Subject: pkix.Name{
					CommonName: "Testing Certificate",
				},
				NotBefore: time.Now(),
				NotAfter:  time.Now().Add(time.Hour * 24 * 180),

				KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
				ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
				BasicConstraintsValid: true,
			}
			derBytes, err := x509.CreateCertificate(insecureRandReader, &template, &template, priv.Public(), priv)
			c.So(err, ShouldBeNil)
			selfSignedPrivateKey = priv
			selfSignedCert = derBytes
		})
		c.Convey("[Test]Setup http2 server", func(c C) {
			listener, err := tls.Listen("tcp", "127.0.0.1:23802", &tls.Config{
				NextProtos: []string{http2.NextProtoTLS},
				Certificates: []tls.Certificate{
					tls.Certificate{Certificate: [][]byte{selfSignedCert}, PrivateKey: selfSignedPrivateKey},
				},
			})
			c.So(err, ShouldBeNil)
			s := http.Server{}
			go s.Serve(listener)
			go func() {
				<-httpServerContext.Done()
				s.Close()
			}()
		})
		c.Convey("[Test]Setup http1 server", func(c C) {
			listener, err := tls.Listen("tcp", "127.0.0.1:23801", &tls.Config{
				NextProtos: []string{"http/1.1"},
				Certificates: []tls.Certificate{
					tls.Certificate{Certificate: [][]byte{selfSignedCert}, PrivateKey: selfSignedPrivateKey},
				},
			})
			c.So(err, ShouldBeNil)
			s := http.Server{}
			go s.Serve(listener)
			go func() {
				<-httpServerContext.Done()
				s.Close()
			}()
		})
	})
	for _, v := range []struct {
		id   utls.ClientHelloID
		name string
	}{
		{
			id:   utls.HelloChrome_58,
			name: "HelloChrome_58",
		},
		{
			id:   utls.HelloChrome_62,
			name: "HelloChrome_62",
		},
		{
			id:   utls.HelloChrome_70,
			name: "HelloChrome_70",
		},
		{
			id:   utls.HelloChrome_72,
			name: "HelloChrome_72",
		},
		{
			id:   utls.HelloChrome_83,
			name: "HelloChrome_83",
		},
		{
			id:   utls.HelloFirefox_55,
			name: "HelloFirefox_55",
		},
		{
			id:   utls.HelloFirefox_55,
			name: "HelloFirefox_55",
		},
		{
			id:   utls.HelloFirefox_63,
			name: "HelloFirefox_63",
		},
		{
			id:   utls.HelloFirefox_65,
			name: "HelloFirefox_65",
		},
		{
			id:   utls.HelloIOS_11_1,
			name: "HelloIOS_11_1",
		},
		{
			id:   utls.HelloIOS_12_1,
			name: "HelloIOS_12_1",
		},
	} {
		t.Run("Testing fingerprint for "+v.name, func(t *testing.T) {
			rtter := NewUTLSHTTPRoundTripper(v.id, &utls.Config{
				InsecureSkipVerify: true,
			}, http.DefaultTransport, false)

			for count := 0; count <= 10; count++ {
				Convey("HTTP 1.1 Test", t, func(c C) {
					{
						req, err := http.NewRequest("GET", "https://127.0.0.1:23801/", nil)
						So(err, ShouldBeNil)
						_, err = rtter.RoundTrip(req)
						So(err, ShouldBeNil)
					}
				})

				Convey("HTTP 2 Test", t, func(c C) {
					{
						req, err := http.NewRequest("GET", "https://127.0.0.1:23802/", nil)
						So(err, ShouldBeNil)
						_, err = rtter.RoundTrip(req)
						So(err, ShouldBeNil)
					}
				})
			}
		})
	}

	cancel()
}
