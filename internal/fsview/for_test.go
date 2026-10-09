package fsview

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func TestForStoppedContainer(t *testing.T) {
	id := strings.Repeat("ab", 32)
	f := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{
		"db":     {ID: id, Name: "db"},
		"zombie": {ID: id, Name: "zombie", Running: true}, // running without a pid
	}}
	for _, name := range []string{"db", "zombie"} {
		if _, err := For(context.Background(), f, name); !errors.Is(err, ErrStopped) {
			t.Errorf("%s : %v, attendu ErrStopped", name, err)
		}
	}
}

func TestForPassesDockerErrors(t *testing.T) {
	if _, err := For(context.Background(), &dockerx.Fake{}, "nope"); !errors.Is(err, dockerx.ErrNotFound) {
		t.Fatalf("inconnu : %v", err)
	}
	down := &dockerx.Fake{Err: fmt.Errorf("%w : dial", dockerx.ErrUnavailable)}
	if _, err := For(context.Background(), down, "web"); !errors.Is(err, dockerx.ErrUnavailable) {
		t.Fatalf("Docker arrêté : %v", err)
	}
}
