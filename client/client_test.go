package main

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestICEServerParser(t *testing.T) {
	Convey("Test parsing of ICE servers", t, func() {
		for _, test := range []struct {
			input  string
			urls   [][]string
			length int
		}{
			{
				"",
				nil,
				0,
			},
			{
				" ",
				nil,
				0,
			},
			{
				"stun:stun.l.google.com:19302",
				[][]string{[]string{"stun:stun.l.google.com:19302"}},
				1,
			},
			{
				"stun:stun.l.google.com:19302,stun.ekiga.net",
				[][]string{[]string{"stun:stun.l.google.com:19302"}, []string{"stun.ekiga.net"}},
				2,
			},
			{
				"stun:stun.l.google.com:19302, stun.ekiga.net",
				[][]string{[]string{"stun:stun.l.google.com:19302"}, []string{"stun.ekiga.net"}},
				2,
			},
		} {
			servers := parseIceServers(test.input)

			if test.urls == nil {
				So(servers, ShouldBeNil)
			} else {
				So(servers, ShouldNotBeNil)
			}

			So(len(servers), ShouldEqual, test.length)

			for i, server := range servers {
				So(server.URLs, ShouldResemble, test.urls[i])
			}

		}

	})
}
