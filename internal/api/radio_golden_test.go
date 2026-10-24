package api

// Golden file of GET /api/containers/{id} (F7): the real output of
// toRadioJSON for a typical stopped container, shared with the interface
// (web/src/lib/xray.test.ts reads the same file). A change of the JSON fails
// here; HUBLOT_UPDATE_GOLDEN=1 rewrites the file, to be reviewed in the diff.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// The golden file lives in the repository, not in a container: plain os
// calls are fine here.
var radioGolden = filepath.Join("testdata", "radio.golden.json")

func goldenRadio() dockerx.ContainerInfo {
	return dockerx.ContainerInfo{
		ID: "c0ffee0123456789", Name: "cache", Running: false,
		FinishedAt: "2026-10-20T08:00:00.123456789Z", // the last exit (spec v0.2 §5.3)
		Mounts: []dockerx.Mount{
			{Type: "volume", Name: "data", Source: "/var/lib/docker/volumes/data/_data", Destination: "/data", Driver: "local", Mode: "ro"},
			{Type: "bind", Source: "/srv/conf", Destination: "/etc/app", RW: true},
		},
		Tmpfs: []string{"/run"}, LogDriver: "json-file",
		Details: dockerx.Details{
			Image: "redis:7", ImageID: "sha256:0123456789abcdef", Created: "2026-10-10T07:00:00Z", Status: "exited",
			ExitCode: 137, StartedAt: "2026-10-10T07:00:05Z",
			// Spec v0.2 §5.2: one label in clear, two masked (outside the
			// whitelist; a secret value under it).
			Labels: map[string]string{
				"com.docker.compose.project":                    "shop",
				"com.docker.compose.x":                          "postgres://u:s3cr3t-label@db/app",
				"traefik.http.middlewares.auth.basicauth.users": "admin:$apr1$s3cr3t-label",
			},
			RestartPolicy: "on-failure", MaxRetry: 5,
			Cmd:        []string{"redis-server", "--requirepass", "s3cr3t-cmd"},
			WorkingDir: "/data",
			Env:        []string{"DB_PASSWORD=s3cr3t-env", "PLAIN=visible"},
			// -p 90 (host port chosen at start), -p [::1]:8080:80, EXPOSE 9000.
			Ports: []dockerx.Port{
				{Private: 80, Public: 8080, Type: "tcp", IP: "::1"},
				{Private: 90, Type: "tcp", IP: "0.0.0.0"},
				{Private: 9000, Type: "tcp"},
			},
			NetworkMode: "bridge",
			Networks:    []dockerx.Network{{Name: "bridge"}},
		},
	}
}

func TestRadioGolden(t *testing.T) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf) // as writeJSON
	enc.SetIndent("", "\t")
	if err := enc.Encode(toRadioJSON(goldenRadio())); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("HUBLOT_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(radioGolden, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(radioGolden)
	if err != nil {
		t.Fatalf("%v (HUBLOT_UPDATE_GOLDEN=1 le crée)", err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("le JSON de la radiographie a changé (l'interface lit %s) :\n%s", radioGolden, buf.String())
	}
	noSecret(t, buf.String())
}
