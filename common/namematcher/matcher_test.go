package namematcher

import "testing"

import . "github.com/smartystreets/goconvey/convey"

func TestMatchMember(t *testing.T) {
	testingVector := []struct {
		matcher string
		target  string
		expects bool
	}{
		{matcher: "", target: "", expects: true},
		{matcher: "^snowflake.torproject.net$", target: "snowflake.torproject.net", expects: true},
		{matcher: "^snowflake.torproject.net$", target: "faketorproject.net", expects: false},
		{matcher: "snowflake.torproject.net$", target: "faketorproject.net", expects: false},
		{matcher: "snowflake.torproject.net$", target: "snowflake.torproject.net", expects: true},
		{matcher: "snowflake.torproject.net$", target: "imaginary-01-snowflake.torproject.net", expects: true},
		{matcher: "snowflake.torproject.net$", target: "imaginary-aaa-snowflake.torproject.net", expects: true},
		{matcher: "snowflake.torproject.net$", target: "imaginary-aaa-snowflake.faketorproject.net", expects: false},
	}
	for _, v := range testingVector {
		t.Run(v.matcher+"<>"+v.target, func(t *testing.T) {
			Convey("test", t, func() {
				matcher := NewNameMatcher(v.matcher)
				So(matcher.IsMember(v.target), ShouldEqual, v.expects)
			})
		})
	}
}

func TestMatchSubset(t *testing.T) {
	testingVector := []struct {
		matcher string
		target  string
		expects bool
	}{
		{matcher: "", target: "", expects: true},
		{matcher: "^snowflake.torproject.net$", target: "^snowflake.torproject.net$", expects: true},
		{matcher: "snowflake.torproject.net$", target: "^snowflake.torproject.net$", expects: true},
		{matcher: "snowflake.torproject.net$", target: "snowflake.torproject.net$", expects: true},
		{matcher: "snowflake.torproject.net$", target: "testing-snowflake.torproject.net$", expects: true},
		{matcher: "snowflake.torproject.net$", target: "^testing-snowflake.torproject.net$", expects: true},
		{matcher: "snowflake.torproject.net$", target: "", expects: false},
	}
	for _, v := range testingVector {
		t.Run(v.matcher+"<>"+v.target, func(t *testing.T) {
			Convey("test", t, func() {
				matcher := NewNameMatcher(v.matcher)
				target := NewNameMatcher(v.target)
				So(matcher.IsSupersetOf(target), ShouldEqual, v.expects)
			})
		})
	}
}
