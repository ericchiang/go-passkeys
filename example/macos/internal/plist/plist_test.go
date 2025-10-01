package plist

import (
	"bytes"
	"testing"
)

func TestMarshal(t *testing.T) {
	testCases := []struct {
		obj  Object
		want string
	}{
		{String("hello"), "<string>hello</string>"},
		{String("<hello/>"), "<string>&lt;hello/&gt;</string>"},
		{Bool(true), "<true/>"},
		{Bool(false), "<false/>"},
		{Int(1), "<integer>1</integer>"},
		{Int(-2344), "<integer>-2344</integer>"},
		{
			Array(
				String("hello"),
				Bool(true),
				Int(1),
				Int(-2344),
			),
			"<array><string>hello</string><true/><integer>1</integer><integer>-2344</integer></array>",
		},
		{
			Dict().Add("a", String("hello")), "<dict><key>a</key><string>hello</string></dict>",
		},
		{
			Dict().Add("<a>", String("hello")), "<dict><key>&lt;a&gt;</key><string>hello</string></dict>",
		},
	}
	for _, tc := range testCases {
		buf := bytes.NewBuffer(nil)
		if err := tc.obj.encodePlist(buf); err != nil {
			t.Errorf("Marshal %#v: %v", tc.obj, err)
			continue
		}
		got := buf.String()
		if string(got) != tc.want {
			t.Errorf("Marshal(%v) = got: %q, want: %q", tc.obj, string(got), tc.want)
		}
	}
}
