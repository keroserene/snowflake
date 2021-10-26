package snowflake_proxy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTokens(t *testing.T) {
	Convey("Tokens", t, func() {
		tokens := newTokens(2)
		So(tokens.count(), ShouldEqual, 0)
		tokens.get()
		So(tokens.count(), ShouldEqual, 1)
		tokens.ret()
		So(tokens.count(), ShouldEqual, 0)
	})
	Convey("Tokens capacity 0", t, func() {
		tokens := newTokens(0)
		So(tokens.count(), ShouldEqual, 0)
		for i := 0; i < 20; i++ {
			tokens.get()
		}
		So(tokens.count(), ShouldEqual, 20)
		tokens.ret()
		So(tokens.count(), ShouldEqual, 19)
	})
}
