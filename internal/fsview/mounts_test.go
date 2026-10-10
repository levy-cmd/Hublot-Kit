package fsview

import (
	"context"
	"errors"
	"io/fs"
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
		"fb":   {"type": "fuseblk.ntfs", "device": "/dev/sdb1"},
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
		{vol("fb", "/fb", "local"), []string{"fb", "type=fuseblk.ntfs", "/fb"}},
		{dockerx.Mount{Type: "cluster", Destination: "/cl"}, []string{"cluster", "/cl"}},
		{dockerx.Mount{Type: "npipe", Destination: "/np"}, []string{"npipe", "/np"}},
		{dockerx.Mount{Type: "", Destination: "/empty"}, []string{"/empty"}},
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
		{Type: "tmpfs", Destination: "/tmp"}, {Type: "image", Destination: "/img"},
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
	// The options field is "-": the separator is one field too early.
	if _, typ, found := PseudoMountUnder("700 600 0:70 / /mnt - ext4 /dev/sdb1 rw\n", "/etc"); !found || !strings.Contains(typ, "illisible") {
		t.Fatalf("séparateur à la place des options accepté : %q, %v", typ, found)
	}
	if _, _, found := PseudoMountUnder("700 600 0:70 / /mnt rw - fuseblk.ntfs /dev/sdb1 rw\n", "/mnt"); !found {
		t.Fatal("fuseblk.<sous-type> est FUSE")
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

// Review fix 1: mountinfo must show "/" and every Docker destination.
func TestCheckVisibleMounts(t *testing.T) {
	m := func(dst string) dockerx.Mount { return dockerx.Mount{Type: "bind", Source: "/srv", Destination: dst} }
	if err := CheckVisibleMounts(sampleMountinfo, []dockerx.Mount{m("/data"), m("/my dir/"), {Type: "tmpfs", Destination: "/dev"}}); err != nil {
		t.Fatalf("cas normal refusé : %v", err)
	}
	if err := CheckVisibleMounts(sampleMountinfo, nil); err != nil {
		t.Fatalf("sans montage Docker : %v", err)
	}
	// Destination missing (moved root, or under a symlink).
	err := CheckVisibleMounts(sampleMountinfo, []dockerx.Mount{m("/link/m")})
	if !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "ne montre pas la destination /link/m") {
		t.Fatalf("destination absente : %v", err)
	}
	// No "/" line: the process root was moved to a jail.
	const jail = "700 600 254:1 / /jail/x rw - ext4 /dev/vda1 rw\n"
	err = CheckVisibleMounts(jail, nil)
	if !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), `pas de racine "/"`) {
		t.Fatalf("pas de racine : %v", err)
	}
	if err := CheckVisibleMounts("garbage\n", nil); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("ligne illisible : %v", err)
	}
}

func TestLocateMounts(t *testing.T) {
	mi := strings.Join([]string{
		"20 1 0:20 / / rw - overlay overlay rw",
		"42 20 0:5 /docker.sock /run/docker.sock rw - tmpfs tmpfs rw",
		"43 20 8:1 /var/lib/docker/volumes/v/_data /run/x rw - ext4 /dev/sda1 rw",
		"44 43 0:4 / /run/x/proc rw - proc proc rw",
	}, "\n")
	ids := map[string]MountIdent{
		"/var/run/docker.sock": {42, 5, 7}, "/run/docker.sock": {42, 5, 7}, // same mount, same inode
		"/var/run/sock2": {42, 5, 7},                     // a second destination onto that mount
		"/var/run/plain": {20, 20, 99}, "/": {20, 20, 2}, // not a mount: on "/", another inode
		"/data":      {20, 20, 2}, // links planted after a chroot lead /data to the root
		"/data2":     {43, 30, 2}, // ... or /data2 to the root of another mount
		"/var/run/x": {43, 30, 2}, "/run/x": {43, 30, 2},
		"/var/run/stale": {77, 1, 1}, // ID absent from the table
		"/var/run/other": {42, 5, 8}, // same mount, another inode
	}
	ident := func(p string) (MountIdent, error) {
		if id, ok := ids[p]; ok {
			return id, nil
		}
		return MountIdent{}, fs.ErrNotExist
	}
	sock := func(dst string) dockerx.Mount {
		return dockerx.Mount{Type: "bind", Source: "/var/run/docker.sock", Destination: dst}
	}

	in := []dockerx.Mount{sock("/var/run/docker.sock")}
	got := LocateMounts(mi, in, ident)
	if got[0].Destination != "/run/docker.sock" || in[0].Destination != "/var/run/docker.sock" {
		t.Fatalf("localisé : %v (entrée %v, qui ne doit pas changer)", got, in)
	}
	if err := CheckVisibleMounts(mi, got); err != nil {
		t.Fatalf("après localisation : %v", err)
	}
	vol := []dockerx.Mount{{Type: "volume", Name: "v", Destination: "/var/run/x"}}
	if _, _, dst, bad := PseudoMountInMounts(mi, LocateMounts(mi, vol, ident)); !bad || dst != "/run/x" {
		t.Errorf("le point retrouvé nourrit la règle « tout le conteneur » : %v %v", bad, dst)
	}
	// Fail closed: each of these stays where it is, and the tar is refused.
	refused := map[string][]dockerx.Mount{
		"pas un montage":               {sock("/var/run/plain")},
		"ID inconnu":                   {sock("/var/run/stale")},
		"autre inode":                  {sock("/var/run/other")},
		"introuvable":                  {sock("/var/run/absent")},
		"chroot vers /":                {{Type: "bind", Source: "/srv", Destination: "/data"}},
		"chroot vers un autre montage": {{Type: "bind", Source: "/srv", Destination: "/data2"}},
		"autre source":                 {{Type: "bind", Source: "/srv/other.sock", Destination: "/var/run/docker.sock"}},
		"autre volume":                 {{Type: "volume", Name: "w", Destination: "/var/run/x"}},
		"tmpfs":                        {{Type: "tmpfs", Destination: "/var/run/docker.sock"}},
		"même point deux fois":         {sock("/var/run/docker.sock"), sock("/var/run/sock2")},
		"point déjà visible":           {sock("/run/docker.sock"), sock("/var/run/docker.sock")},
	}
	for name, ms := range refused {
		if err := CheckVisibleMounts(mi, LocateMounts(mi, ms, ident)); !errors.Is(err, ErrForbiddenPath) {
			t.Errorf("ÉVASION : %s accepté", name)
		}
	}
	// A visible destination is never resolved; an unreadable ID locates nothing.
	LocateMounts(mi, []dockerx.Mount{sock("/run/docker.sock")}, func(string) (MountIdent, error) {
		t.Error("ident appelé pour une destination visible")
		return MountIdent{}, nil
	})
	if got := LocateMounts("x 20 0:20 / / rw - overlay overlay rw", in, ident); got[0].Destination != "/var/run/docker.sock" {
		t.Errorf("ID illisible : %v", got)
	}
}

// Review I1: PID 1 chrooted into a bind (/srv/data mounted at /app), a link
// app -> / leads the destination to the root mount, whose root ends the
// bind's Source. Only the "P is not /" guard refuses it.
func TestLocateMountsChrootToRootMountEndingSource(t *testing.T) {
	mi := "20 1 8:1 /srv/data / rw - ext4 /dev/sda1 rw"
	ident := func(p string) (MountIdent, error) {
		if p == "/data" || p == "/" {
			return MountIdent{20, 30, 2}, nil
		}
		return MountIdent{}, fs.ErrNotExist
	}
	ms := []dockerx.Mount{{Type: "bind", Source: "/srv/data", Destination: "/data"}}
	got := LocateMounts(mi, ms, ident)
	if got[0].Destination != "/data" {
		t.Errorf("ÉVASION : destination remplacée par la racine : %v", got)
	}
	if err := CheckVisibleMounts(mi, got); !errors.Is(err, ErrForbiddenPath) {
		t.Errorf("ÉVASION : accepté (%v)", err)
	}
}

// Spec §13: a --tmpfs whose destination passes through a link (compose
// "tmpfs: [/var/run]", /var/run -> ../run) is mounted at /run, so the table
// does not show /var/run. It is found by mount ID like a bind or a volume,
// with its own tie: a whole tmpfs (fstype tmpfs, root "/"), never on "/",
// never on a point another destination already holds, never on a mount
// the runtime adds (/dev, /dev/shm, /proc, /sys).
func TestLocateMountsTmpfs(t *testing.T) {
	mi := strings.Join([]string{
		"20 1 0:20 / / rw - overlay overlay rw",
		"50 20 0:50 / /run rw - tmpfs tmpfs rw",
		"51 20 0:51 / /scratch rw - tmpfs tmpfs rw",
		"52 20 0:52 / /dev rw - tmpfs tmpfs rw",
		"53 52 0:53 / /dev/shm rw - tmpfs shm rw",
		"54 20 0:54 / /hostrun rw - tmpfs tmpfs rw",
		"55 20 8:1 /var/lib/docker/volumes/v/_data /data rw - ext4 /dev/sda1 rw",
		"56 20 0:56 /sub /part rw - tmpfs tmpfs rw",
	}, "\n")
	ids := map[string]MountIdent{
		"/run": {50, 50, 1}, "/var/run": {50, 50, 1}, // the case of spec §13
		"/scratch": {51, 51, 1}, "/to-scratch": {51, 51, 1}, // decoy: another declared tmpfs
		"/dev": {52, 52, 1}, "/to-dev": {52, 52, 1}, // decoy: runtime mounts
		"/dev/shm": {53, 53, 1}, "/to-shm": {53, 53, 1},
		"/hostrun": {54, 54, 1}, "/to-hostrun": {54, 54, 1}, // a bind of a host tmpfs
		"/data": {55, 30, 2}, "/to-data": {55, 30, 2}, // not a tmpfs
		"/part": {56, 56, 1}, "/to-part": {56, 56, 1}, // a part of a tmpfs, not a whole one
		"/": {20, 20, 2}, "/to-root": {20, 20, 2}, // jail: the link leads to the root
		"/other-inode": {50, 50, 9}, // same mount, another inode
		"/var/run2":    {50, 50, 1},
	}
	ident := func(p string) (MountIdent, error) {
		if id, ok := ids[p]; ok {
			return id, nil
		}
		return MountIdent{}, fs.ErrNotExist
	}
	tmpfs := func(dst string) dockerx.Mount { return dockerx.Mount{Type: "tmpfs", Destination: dst} }

	got := LocateMounts(mi, []dockerx.Mount{tmpfs("/var/run"), tmpfs("/scratch")}, ident)
	if got[0].Destination != "/run" || got[1].Destination != "/scratch" {
		t.Fatalf("tmpfs sous un lien : %v, attendu /run", got)
	}
	if err := CheckVisibleMounts(mi, got); err != nil {
		t.Fatalf("après localisation : %v", err)
	}
	// Fail closed: each of these stays where it is, and the write is refused.
	refused := map[string][]dockerx.Mount{
		"lien vers la racine (overlay)":       {tmpfs("/to-root")},
		"leurre : un autre tmpfs déclaré":     {tmpfs("/to-scratch"), tmpfs("/scratch")},
		"leurre : /dev du runtime":            {tmpfs("/to-dev")},
		"leurre : /dev/shm du runtime":        {tmpfs("/to-shm")},
		"lien vers un autre montage (bind)":   {tmpfs("/to-hostrun"), {Type: "bind", Source: "/run", Destination: "/hostrun"}},
		"lien vers un volume":                 {tmpfs("/to-data"), {Type: "volume", Name: "v", Destination: "/data"}},
		"partie d'un tmpfs":                   {tmpfs("/to-part")},
		"autre inode":                         {tmpfs("/other-inode")},
		"introuvable":                         {tmpfs("/absent")},
		"deux destinations sur un même tmpfs": {tmpfs("/var/run"), tmpfs("/var/run2")},
	}
	for name, ms := range refused {
		if err := CheckVisibleMounts(mi, LocateMounts(mi, ms, ident)); !errors.Is(err, ErrForbiddenPath) {
			t.Errorf("ÉVASION : %s accepté", name)
		}
	}
	// Jail: PID 1 chrooted into the tmpfs itself, with a link var/run -> /
	// planted in it: the destination leads to the jail's root, a whole tmpfs
	// on "/". Only the "P is not /" guard refuses it.
	jail := "60 1 0:60 / / rw - tmpfs tmpfs rw"
	jailIdent := func(p string) (MountIdent, error) {
		if p == "/" || p == "/var/run" {
			return MountIdent{60, 60, 1}, nil
		}
		return MountIdent{}, fs.ErrNotExist
	}
	if err := CheckVisibleMounts(jail, LocateMounts(jail, []dockerx.Mount{tmpfs("/var/run")}, jailIdent)); !errors.Is(err, ErrForbiddenPath) {
		t.Errorf("ÉVASION : prison dans le tmpfs acceptée")
	}
	// The tie of a bind or a volume never accepts a tmpfs line, nor the
	// reverse: a bind whose destination leads to /run stays refused.
	bind := []dockerx.Mount{{Type: "bind", Source: "/srv/run", Destination: "/var/run"}}
	if err := CheckVisibleMounts(mi, LocateMounts(mi, bind, ident)); !errors.Is(err, ErrForbiddenPath) {
		t.Errorf("ÉVASION : un bind localisé sur un tmpfs")
	}
}

// Review I1: a whole tmpfs that no Docker mount declares can still be a
// decoy when a declared bind brings it (rbind of a host tree with
// submounts, here /hostrun/lock) or when the container mounts it itself
// under a volume. runc mounts a --tmpfs directly on the container's root
// filesystem: the parent of its line (field 2) must be a "/" line.
func TestLocateMountsTmpfsParentIsRoot(t *testing.T) {
	tmpfs := func(dst string) dockerx.Mount { return dockerx.Mount{Type: "tmpfs", Destination: dst} }
	identOf := func(ids map[string]MountIdent) func(string) (MountIdent, error) {
		return func(p string) (MountIdent, error) {
			if id, ok := ids[p]; ok {
				return id, nil
			}
			return MountIdent{}, fs.ErrNotExist
		}
	}
	// A, no jail: /var/run -> /hostrun/lock, a tmpfs under the bind of /run.
	a := strings.Join([]string{
		"20 1 0:20 / / rw - overlay overlay rw",
		"30 20 0:30 / /hostrun rw - tmpfs tmpfs rw",
		"31 30 0:31 / /hostrun/lock rw - tmpfs tmpfs rw",
	}, "\n")
	mountsA := []dockerx.Mount{tmpfs("/var/run"), {Type: "bind", Source: "/run", Destination: "/hostrun"}}
	identA := identOf(map[string]MountIdent{
		"/var/run": {31, 31, 1}, "/hostrun/lock": {31, 31, 1}, "/hostrun": {30, 30, 1},
	})
	if err := CheckVisibleMounts(a, LocateMounts(a, mountsA, identA)); !errors.Is(err, ErrForbiddenPath) {
		t.Errorf("ÉVASION : tmpfs d'un sous-montage de bind pris pour le --tmpfs (%v)", err)
	}
	// B, jail: PID 1 chrooted in the tmpfs /jail, which holds a bind of /srv
	// whose submount /srv/cache is a whole tmpfs. The table shows the jail's
	// "/", /srv and /srv/cache; /jail is not there, and must not be found
	// on /srv/cache.
	b := strings.Join([]string{
		"60 1 0:60 / / rw - tmpfs tmpfs rw",
		"61 60 8:1 /srv /srv rw - ext4 /dev/sda1 rw",
		"62 61 0:62 / /srv/cache rw - tmpfs tmpfs rw",
	}, "\n")
	mountsB := []dockerx.Mount{tmpfs("/jail"), {Type: "bind", Source: "/srv", Destination: "/srv"}}
	identB := identOf(map[string]MountIdent{
		"/jail": {62, 62, 1}, "/srv/cache": {62, 62, 1}, "/srv": {61, 8, 2},
	})
	if err := CheckVisibleMounts(b, LocateMounts(b, mountsB, identB)); !errors.Is(err, ErrForbiddenPath) {
		t.Errorf("ÉVASION : prison avec un tmpfs sous un bind acceptée (%v)", err)
	}
	// Control: the same tmpfs, child of the root, is found.
	c := strings.Join([]string{
		"20 1 0:20 / / rw - overlay overlay rw",
		"50 20 0:50 / /run rw - tmpfs tmpfs rw",
	}, "\n")
	identC := identOf(map[string]MountIdent{"/var/run": {50, 50, 1}, "/run": {50, 50, 1}})
	if got := LocateMounts(c, []dockerx.Mount{tmpfs("/var/run")}, identC); got[0].Destination != "/run" {
		t.Errorf("contrôle : tmpfs fils de la racine non retrouvé (%v)", got)
	}
}
