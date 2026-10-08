package dockerx

import (
	"net/netip"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
)

func TestFromSummary(t *testing.T) {
	s := container.Summary{
		ID:      "0123456789abcdef",
		Names:   []string{"/web"},
		Image:   "nginx:1.27",
		Created: 1_760_000_000,
		State:   container.ContainerState("running"),
		Status:  "Up 3 hours",
		Ports: []container.PortSummary{
			{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			{IP: netip.MustParseAddr("::"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			{PrivatePort: 443, Type: "tcp"},
		},
	}
	c := fromSummary(s)
	if c.ID != s.ID || c.Name != "web" || c.Image != "nginx:1.27" || c.State != "running" || c.Status != "Up 3 hours" {
		t.Fatalf("bad mapping: %+v", c)
	}
	if !c.Created.Equal(time.Unix(1_760_000_000, 0)) {
		t.Fatalf("created = %v", c.Created)
	}
	if len(c.Ports) != 2 {
		t.Fatalf("IPv4/IPv6 duplicate must collapse: %+v", c.Ports)
	}
	if c.Ports[0] != (Port{Private: 80, Public: 8080, Type: "tcp", IP: "0.0.0.0"}) || c.Ports[1] != (Port{Private: 443, Type: "tcp"}) {
		t.Fatalf("ports = %+v", c.Ports)
	}
}

func TestFromSummaryWithoutName(t *testing.T) {
	c := fromSummary(container.Summary{ID: "0123456789abcdef0123"})
	if c.Name != "0123456789ab" {
		t.Fatalf("name falls back to the short id, got %q", c.Name)
	}
	if c.Ports == nil {
		t.Fatal("ports must be an empty slice, not nil (JSON [])")
	}
}
