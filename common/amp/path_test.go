package amp

import (
	"testing"
)

func TestDecodePath(t *testing.T) {
	for _, test := range []struct {
		path           string
		expectedData   string
		expectedErrStr string
	}{
		{"", "", "missing format indicator"},
		{"0", "", "missing data"},
		{"0foobar", "", "missing data"},
		{"/0/YWJj", "", "unknown format indicator '/'"},

		{"0/", "", ""},
		{"0foobar/", "", ""},
		{"0/YWJj", "abc", ""},
		{"0///YWJj", "abc", ""},
		{"0foobar/YWJj", "abc", ""},
		{"0/foobar/YWJj", "abc", ""},
	} {
		data, err := DecodePath(test.path)
		if test.expectedErrStr != "" {
			if err == nil || err.Error() != test.expectedErrStr {
				t.Errorf("%+q expected error %+q, got %+q",
					test.path, test.expectedErrStr, err)
			}
		} else if err != nil {
			t.Errorf("%+q expected no error, got %+q", test.path, err)
		} else if string(data) != test.expectedData {
			t.Errorf("%+q expected data %+q, got %+q",
				test.path, test.expectedData, data)
		}
	}
}

func TestPathRoundTrip(t *testing.T) {
	for _, data := range []string{
		"",
		"\x00",
		"/",
		"hello world",
	} {
		decoded, err := DecodePath(EncodePath([]byte(data)))
		if err != nil {
			t.Errorf("%+q roundtripped with error %v", data, err)
		} else if string(decoded) != data {
			t.Errorf("%+q roundtripped to %+q", data, decoded)
		}
	}
}
