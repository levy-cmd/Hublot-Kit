//go:build linux

package procroot

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestOpenRejectsForeignCgroup(t *testing.T) {
	_, err := Open(os.Getpid(), strings.Repeat("a", 64))
	if !errors.Is(err, ErrChanged) {
		t.Fatalf("want ErrChanged, got %v", err)
	}
}

func TestOpenRejectsMissingPid(t *testing.T) {
	_, err := Open(0x7ffffffe, strings.Repeat("a", 64))
	if !errors.Is(err, ErrChanged) {
		t.Fatalf("want ErrChanged, got %v", err)
	}
}
