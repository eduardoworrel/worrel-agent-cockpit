package main

import "testing"

func TestBrowserCommand(t *testing.T) {
	cases := []struct {
		goos, wantName, wantFirstArg string
	}{
		{"darwin", "open", "http://x"},
		{"windows", "rundll32", "url.dll,FileProtocolHandler"},
		{"linux", "xdg-open", "http://x"},
	}
	for _, c := range cases {
		name, args := browserCommand(c.goos, "http://x")
		if name != c.wantName {
			t.Errorf("%s: name=%s want %s", c.goos, name, c.wantName)
		}
		if len(args) == 0 || args[0] != c.wantFirstArg {
			t.Errorf("%s: args=%v want first %s", c.goos, args, c.wantFirstArg)
		}
	}
}
