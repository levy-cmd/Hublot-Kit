package fsview

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func TestHostBind(t *testing.T) {
	ctx := context.Background()
	docker := &dockerx.Fake{Volumes: map[string]map[string]string{
		"proc-vol":  {"type": "none", "o": "bind", "device": "/proc"},
		"sys-vol":   {"o": "rbind,ro", "device": "/sys/kernel"},
		"proc-type": {"type": "proc", "device": "proc"},
		"sshfs":     {"type": "fuse.sshfs", "device": "host:/"},
		"plain-vol": {},
		"srv-vol":   {"type": "none", "o": "bind", "device": "/srv/data"},
		"nfs-vol":   {"type": "nfs", "o": "addr=10.0.0.1", "device": ":/export"},
		"rel-vol":   {"type": "none", "o": "bind", "device": "proc"},
		// Readable options: only the driver rule may refuse these two.
		"ceph": {},
		"anon": {},
	}}
	bind := func(src, dst string) dockerx.Mount { return dockerx.Mount{Type: "bind", Source: src, Destination: dst} }
	vol := func(name, dst, driver string) dockerx.Mount {
		return dockerx.Mount{Type: "volume", Name: name, Destination: dst, Driver: driver}
	}
	refused := []struct {
		m    dockerx.Mount
		want []string // the error names the mount
	}{
		{bind("/", "/host"), []string{"bind / -> /host"}},
		{bind("/proc", "/host/proc"), []string{"bind /proc -> /host/proc"}},
		{bind("/proc/1/ns", "/ns"), []string{"bind /proc/1/ns -> /ns"}},
		{bind("/sys", "/host/sys"), []string{"bind /sys -> /host/sys"}},
		{bind("/sys/fs/cgroup", "/cg"), []string{"bind /sys/fs/cgroup -> /cg"}},
		{bind("/dev", "/host/dev"), []string{"bind /dev -> /host/dev"}},
		{bind("/proc/", "/p"), []string{"bind /proc/ -> /p"}},
		{vol("proc-vol", "/data", "local"), []string{"bind /proc -> /data", "proc-vol"}},
		{vol("sys-vol", "/data", "local"), []string{"bind /sys/kernel -> /data", "sys-vol"}},
		{vol("proc-type", "/p", "local"), []string{"proc-type", "type=proc", "/p"}},
		{vol("sshfs", "/remote", "local"), []string{"sshfs", "type=fuse.sshfs", "/remote"}},
		{vol("ceph", "/c", "rexray/rbd"), []string{"ceph", "/c", "rexray/rbd"}},
		{vol("anon", "/a", ""), []string{"anon", "/a"}},
		{vol("gone", "/g", "local"), []string{"gone", "/g", "options illisibles"}},
		{bind("proc", "/rel"), []string{"bind proc -> /rel"}},                   // not absolute: fail closed
		{vol("rel-vol", "/r", "local"), []string{"bind proc -> /r", "rel-vol"}}, // idem for a device
	}
	for _, c := range refused {
		err := HostBind(ctx, docker, []dockerx.Mount{bind("/srv", "/srv"), c.m})
		if !errors.Is(err, ErrForbiddenPath) {
			t.Errorf("%+v : %v, attendu ErrForbiddenPath", c.m, err)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%+v : %q ne nomme pas %q", c.m, err, w)
			}
		}
	}
	allowed := []dockerx.Mount{
		bind("/srv/data", "/data"), bind("/dev/log", "/dev/log"), bind("/dev/null", "/x"),
		bind("/procfs-backup", "/b"), bind("/system", "/s"),
		vol("plain-vol", "/v", "local"), vol("srv-vol", "/w", "local"), vol("nfs-vol", "/n", "local"),
		{Type: "tmpfs", Destination: "/tmp"},
	}
	if err := HostBind(ctx, docker, allowed); err != nil {
		t.Fatalf("montages ordinaires refusés : %v", err)
	}
	if err := HostBind(ctx, nil, []dockerx.Mount{vol("plain-vol", "/v", "local")}); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("sans client Docker, un volume doit être refusé : %v", err)
	}
}

const sampleMountinfo = `600 500 0:50 / / rw,relatime master:1 - overlay overlay rw,lowerdir=/l
601 600 0:52 / /proc rw,nosuid,nodev,noexec,relatime - proc proc rw
602 600 0:53 / /dev rw,nosuid - tmpfs tmpfs rw,size=65536k,mode=755
603 602 0:54 / /dev/pts rw,nosuid,noexec,relatime - devpts devpts rw,gid=5,mode=620
604 600 0:55 / /sys ro,nosuid,nodev,noexec,relatime - sysfs sysfs ro
605 604 0:26 / /sys/fs/cgroup ro,nosuid,nodev,noexec,relatime - cgroup2 cgroup rw
606 600 254:1 /docker/volumes/x/_data /data rw,relatime - ext4 /dev/vda1 rw
607 606 0:60 / /data/remote rw,nosuid,nodev,relatime - fuse.sshfs host:/ rw,user_id=0
608 600 0:61 / /host/proc rw,relatime - proc proc rw
609 600 254:1 /srv /my\040dir rw,relatime - ext4 /dev/vda1 rw
610 609 0:62 / /my\040dir/sys rw,relatime - sysfs sysfs rw
`

func TestPseudoMountUnder(t *testing.T) {
	cases := []struct {
		dir, mountPoint, fsType string
		found                   bool
	}{
		{"/", "/proc", "proc", true},
		{"/etc", "", "", false},
		{"/dev", "", "", false}, // tmpfs, devpts: harmless
		{"/sys", "/sys", "sysfs", true},
		{"/sys/fs", "/sys", "sysfs", true}, // below a pseudo mount
		{"/data", "/data/remote", "fuse.sshfs", true},
		{"/data/other", "", "", false},
		{"/host", "/host/proc", "proc", true},
		{"/host/proc/1", "/host/proc", "proc", true},
		{"/hostfoo", "", "", false},
		{"/my dir", "/my dir/sys", "sysfs", true},
		{"/proc/../etc", "", "", false}, // cleaned: /etc
	}
	for _, c := range cases {
		mp, typ, found := PseudoMountUnder(sampleMountinfo, c.dir)
		if found != c.found || mp != c.mountPoint || typ != c.fsType {
			t.Errorf("%q : (%q, %q, %v), attendu (%q, %q, %v)", c.dir, mp, typ, found, c.mountPoint, c.fsType, c.found)
		}
	}
	if _, _, found := PseudoMountUnder("garbage line\n", "/etc"); !found {
		t.Fatal("une ligne illisible doit refuser (échec fermé)")
	}
	if _, _, found := PseudoMountUnder("700 600 0:70 / /mnt rw - fuseblk /dev/sdb1 rw\n", "/"); !found {
		t.Fatal("fuseblk sous / doit refuser")
	}
}

// Review R5: stacked mounts, propagation fields, escapes.
func TestPseudoMountUnderEdgeCases(t *testing.T) {
	const mi = `611 600 254:1 / /stack rw,relatime - ext4 /dev/vda1 rw
612 611 0:63 / /stack rw,relatime - proc proc rw
613 612 0:64 / /stack rw,relatime - tmpfs tmpfs rw
614 600 0:65 / /prop rw,relatime shared:5 master:1 propagate_from:2 - sysfs sysfs rw
615 600 0:66 / /t\011ab rw - proc proc rw
616 600 0:67 / /n\012l rw - proc proc rw
617 600 0:68 / /b\134s rw - proc proc rw
`
	cases := []struct{ dir, mountPoint, fsType string }{
		{"/stack", "/stack", "proc"}, // hidden under a tmpfs: still counted
		{"/prop", "/prop", "sysfs"},  // optional fields before "-"
		{"/t\tab", "/t\tab", "proc"},
		{"/n\nl", "/n\nl", "proc"},
		{"/b\\s", "/b\\s", "proc"},
	}
	for _, c := range cases {
		mp, typ, found := PseudoMountUnder(mi, c.dir)
		if !found || mp != c.mountPoint || typ != c.fsType {
			t.Errorf("%q : (%q, %q, %v), attendu (%q, %q, true)", c.dir, mp, typ, found, c.mountPoint, c.fsType)
		}
	}
	for _, bad := range []string{`/bad\08x`, `/bad\9`, `/end\`, `/short\04`} {
		line := "700 600 254:1 / " + bad + " rw - ext4 /dev/vda1 rw\n"
		if _, typ, found := PseudoMountUnder(line, "/etc"); !found || !strings.Contains(typ, "échappement") {
			t.Errorf("%q : échappement invalide accepté (%q, %v)", bad, typ, found)
		}
	}
}

// Review R2: a dangerous mount under a Docker mount refuses the whole container.
func TestPseudoMountInMounts(t *testing.T) {
	m := func(dst string) dockerx.Mount { return dockerx.Mount{Type: "bind", Source: "/srv", Destination: dst} }
	cases := []struct {
		mounts                []dockerx.Mount
		mountPoint, fsType, d string
		found                 bool
	}{
		{[]dockerx.Mount{m("/etc/app")}, "", "", "", false},
		{[]dockerx.Mount{m("/etc/app"), m("/host")}, "/host/proc", "proc", "/host", true},
		{[]dockerx.Mount{m("/data/")}, "/data/remote", "fuse.sshfs", "/data", true},
		{[]dockerx.Mount{m("/host/proc")}, "/host/proc", "proc", "/host/proc", true},
		{[]dockerx.Mount{m("/hostfoo")}, "", "", "", false},
		{nil, "", "", "", false}, // the container's own /proc is not under any Docker mount
	}
	for _, c := range cases {
		mp, typ, d, found := PseudoMountInMounts(sampleMountinfo, c.mounts)
		if found != c.found || mp != c.mountPoint || typ != c.fsType || d != c.d {
			t.Errorf("%+v : (%q, %q, %q, %v)", c.mounts, mp, typ, d, found)
		}
	}
}
