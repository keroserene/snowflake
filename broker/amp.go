package main

import (
	"log"
	"net/http"
	"strings"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/amp"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/messages"
)

// ampClientOffers is the AMP-speaking endpoint for client poll messages,
// intended for access via an AMP cache. In contrast to the other clientOffers,
// the client's encoded poll message is stored in the URL path rather than the
// HTTP request body (because an AMP cache does not support POST), and the
// encoded client poll response is sent back as AMP-armored HTML.
func ampClientOffers(i *IPC, w http.ResponseWriter, r *http.Request) {
	// The encoded client poll message immediately follows the /amp/client/
	// path prefix, so this function unfortunately needs to be aware of and
	// remote its own routing prefix.
	path := strings.TrimPrefix(r.URL.Path, "/amp/client/")
	if path == r.URL.Path {
		// The path didn't start with the expected prefix. This probably
		// indicates an internal bug.
		log.Println("ampClientOffers: unexpected prefix in path")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var encPollReq []byte
	var response []byte
	var err error

	encPollReq, err = amp.DecodePath(path)
	if err == nil {
		arg := messages.Arg{
			Body:       encPollReq,
			RemoteAddr: "",
		}
		err = i.ClientOffers(arg, &response)
	} else {
		response, err = (&messages.ClientPollResponse{
			Error: "cannot decode URL path",
		}).EncodePollResponse()
	}

	if err != nil {
		// We couldn't even construct a JSON object containing an error
		// message :( Nothing to do but signal an error at the HTTP
		// layer. The AMP cache will translate this 500 status into a
		// 404 status.
		// https://amp.dev/documentation/guides-and-tutorials/learn/amp-caches-and-cors/amp-cache-urls/#redirect-%26-error-handling
		log.Printf("ampClientOffers: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	// Attempt to hint to an AMP cache not to waste resources caching this
	// document. "The Google AMP Cache considers any document fresh for at
	// least 15 seconds."
	// https://developers.google.com/amp/cache/overview#google-amp-cache-updates
	w.Header().Set("Cache-Control", "max-age=15")
	w.WriteHeader(http.StatusOK)

	enc, err := amp.NewArmorEncoder(w)
	if err != nil {
		log.Printf("amp.NewArmorEncoder: %v", err)
		return
	}
	defer enc.Close()

	if _, err := enc.Write(response); err != nil {
		log.Printf("ampClientOffers: unable to write answer: %v", err)
	}
}
