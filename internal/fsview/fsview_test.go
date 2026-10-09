package fsview

import (
	"io/fs"
	"testing"
)

func TestModeFromUnixSpecialBits(t *testing.T) {
	cases := []struct {
		name string
		in   uint32
		want fs.FileMode
	}{
		{"permissions only, type bits dropped", 0o100644, 0o644},
		{"setuid", 0o104755, fs.ModeSetuid | 0o755},
		{"setgid", 0o102755, fs.ModeSetgid | 0o755},
		{"sticky", 0o41777, fs.ModeSticky | 0o777},
		{"all three", 0o7777, fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky | 0o777},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ModeFromUnix(c.in); got != c.want {
				t.Fatalf("ModeFromUnix(%#o) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestUnixPermRoundTrip(t *testing.T) {
	for _, m := range []uint32{0, 0o644, 0o755, 0o4755, 0o2755, 0o1777, 0o7777} {
		if got := UnixPerm(ModeFromUnix(m)); got != m {
			t.Errorf("UnixPerm(ModeFromUnix(%#o)) = %#o", m, got)
		}
	}
}

func TestCleanPath(t *testing.T) {
	cases := map[string]string{
		"":              "/",
		"/":             "/",
		"etc":           "/etc",
		"/etc/":         "/etc",
		"//etc//passwd": "/etc/passwd",
		"/a/./b":        "/a/b",
		"../../etc":     "/etc",
		"/a/../../b":    "/b",
	}
	for in, want := range cases {
		if got := CleanPath(in); got != want {
			t.Errorf("CleanPath(%q) = %q, want %q", in, got, want)
		}
	}
}
