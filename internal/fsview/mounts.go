package fsview

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// Docker builds GET /archive of a directory with the container's binds and
// volumes mounted in it: a bind of the host's /proc puts /proc/kcore (hundreds
// of TiB) in the tar and locks dockerd (measured in review). Checks run before
// every GetArchive of a directory (spec §5.2 step 7, §5.4).

// HostBind refuses a container whose mounts may expose the host to the tar
// (spec §5.2 step 7), with an ErrForbiddenPath that names the mount:
//   - a bind of the host's /, /proc, /sys or a path below them, or of /dev
//     exactly;
//   - a "local" volume with o=bind/rbind and device=… (a bind in disguise,
//     judged by its device), or whose type=… is a pseudo filesystem or FUSE;
//   - a volume of any other driver (unknown: refused, fail closed);
//   - a volume whose options cannot be read (fail closed).
func HostBind(ctx context.Context, docker dockerx.Client, mounts []dockerx.Mount) error {
	for _, m := range mounts {
		switch m.Type {
		case "bind":
			if hostSensitive(m.Source) {
				return fmt.Errorf("%w : bind %s -> %s", ErrForbiddenPath, m.Source, m.Destination)
			}
		case "tmpfs", "image":
		case "volume":
			if m.Driver != "local" {
				return fmt.Errorf("%w : volume %s -> %s : pilote %q inconnu, refusé", ErrForbiddenPath, m.Name, m.Destination, m.Driver)
			}
			if docker == nil {
				return fmt.Errorf("%w : volume %s -> %s : options illisibles (pas de client Docker)", ErrForbiddenPath, m.Name, m.Destination)
			}
			opts, err := docker.VolumeOptions(ctx, m.Name)
			if err != nil {
				return fmt.Errorf("%w : volume %s -> %s : options illisibles (%v)", ErrForbiddenPath, m.Name, m.Destination, err)
			}
			if t := opts["type"]; dangerousFSType(t) {
				return fmt.Errorf("%w : volume %s (type=%s) -> %s", ErrForbiddenPath, m.Name, t, m.Destination)
			}
			if hasBindOption(opts["o"]) && opts["device"] != "" && hostSensitive(opts["device"]) {
				return fmt.Errorf("%w : bind %s -> %s (volume %s)", ErrForbiddenPath, opts["device"], m.Destination, m.Name)
			}
		default:
			return fmt.Errorf("%w : montage de type %q inconnu -> %s, refusé", ErrForbiddenPath, m.Type, m.Destination)
		}
	}
	return nil
}

func hasBindOption(o string) bool {
	for _, opt := range strings.Split(o, ",") {
		if opt = strings.TrimSpace(opt); opt == "bind" || opt == "rbind" {
			return true
		}
	}
	return false
}

// hostSensitive works on a host path string (never opened). A path that is
// not absolute cannot be judged: refused (fail closed).
func hostSensitive(src string) bool {
	if !path.IsAbs(src) {
		return true
	}
	s := path.Clean(src)
	return s == "/" || s == "/dev" ||
		s == "/proc" || strings.HasPrefix(s, "/proc/") ||
		s == "/sys" || strings.HasPrefix(s, "/sys/")
}

// pseudoFSNames are the mountinfo names of the 15 pseudo filesystems of
// spec §5.1 step 9 (pseudofs_linux.go lists their magic numbers).
var pseudoFSNames = []string{
	"proc", "sysfs", "cgroup", "cgroup2", "debugfs", "tracefs", "securityfs", "bpf",
	"pstore", "efivarfs", "binfmt_misc", "selinuxfs", "smackfs", "configfs", "fusectl",
}

// dangerousFSType: a pseudo filesystem, or FUSE (a server that never answers
// would block dockerd while it builds the tar, spec §13).
func dangerousFSType(t string) bool {
	return slices.Contains(pseudoFSNames, t) || t == "fuse" || t == "fuseblk" || strings.HasPrefix(t, "fuse.") || strings.HasPrefix(t, "fuseblk.")
}

// mountLine is the part of a mountinfo line the checks use: fields 1, 4 and 5
// (unescaped) and the filesystem type.
type mountLine struct {
	ID                  uint64
	Root, Point, FSType string
}

// scanMountinfo parses /proc/<pid>/mountinfo (mount points relative to the
// process's root) once, for every check. visit gets each line and returns true
// to stop. A line that cannot be parsed, with an unreadable ID or an invalid
// escape, stops the scan too: fail closed.
func scanMountinfo(mountinfo string, visit func(mountLine) bool) (mountPoint, fsType string, stopped bool) {
	for _, line := range strings.Split(mountinfo, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// id parent major:minor root mount-point options [optional fields…] - type source super-options
		f := strings.Fields(line)
		sep := slices.Index(f, "-")
		if len(f) < 7 || sep < 6 || sep+1 >= len(f) {
			return line, "ligne de mountinfo illisible", true
		}
		id, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			return f[0], "identifiant de montage illisible", true
		}
		root, ok := unescapeMountinfo(f[3])
		if !ok {
			return f[3], "échappement de mountinfo invalide", true
		}
		mp, ok := unescapeMountinfo(f[4])
		if !ok {
			return f[4], "échappement de mountinfo invalide", true
		}
		if visit(mountLine{ID: id, Root: root, Point: mp, FSType: f[sep+1]}) {
			return mp, f[sep+1], true
		}
	}
	return "", "", false
}

// dangerousMount returns the first mount of a dangerous type whose mount point
// matches. Stacked mounts are all read.
func dangerousMount(mountinfo string, match func(mountPoint string) bool) (mountPoint, fsType string, found bool) {
	return scanMountinfo(mountinfo, func(l mountLine) bool { return dangerousFSType(l.FSType) && match(l.Point) })
}

// mountPoints is the set of mount points of the table; an unreadable line is
// ErrForbiddenPath (fail closed).
func mountPoints(mountinfo string) (map[string]bool, error) {
	points := map[string]bool{}
	if mp, typ, bad := scanMountinfo(mountinfo, func(l mountLine) bool { points[l.Point] = true; return false }); bad {
		return nil, fmt.Errorf("%w : %s : %s", ErrForbiddenPath, typ, mp)
	}
	return points, nil
}

// CheckRootMount is the root part of CheckVisibleMounts alone: the table has
// a "/" mount, else the root of the process is not the container's root (a
// chroot in PID 1) and the tree it shows is not the one Docker writes in.
func CheckRootMount(mountinfo string) error {
	points, err := mountPoints(mountinfo)
	if err != nil {
		return err
	}
	if !points["/"] {
		return fmt.Errorf("%w : la table des montages n'a pas de racine \"/\" : la racine du process n'est pas celle du conteneur (chroot du PID 1), écriture refusée", ErrForbiddenPath)
	}
	return nil
}

// rootMountIDs: the mount IDs of the lines mounted on "/" (stacked mounts
// give several).
func rootMountIDs(mountinfo string) []uint64 {
	var ids []uint64
	scanMountinfo(mountinfo, func(l mountLine) bool {
		if l.Point == "/" {
			ids = append(ids, l.ID)
		}
		return false
	})
	return ids
}

// CheckVisibleMounts verifies that the mount table describes the container in
// the coordinates dockerd uses: it must have a "/" mount (else the process's
// root was moved and mounts outside it vanished), and every Docker mount
// destination must appear verbatim as a mount point (else a symlink in the
// path, or a moved root, hides it; LocateMounts may have moved it to the mount
// point it proved to be the same Docker mount). Otherwise the other checks could miss a
// dangerous mount: fail closed.
func CheckVisibleMounts(mountinfo string, mounts []dockerx.Mount) error {
	points, err := mountPoints(mountinfo)
	if err != nil {
		return err
	}
	if !points["/"] {
		return fmt.Errorf("%w : la table des montages n'a pas de racine \"/\" : tar refusé", ErrForbiddenPath)
	}
	for _, m := range mounts {
		if d := CleanPath(m.Destination); !points[d] {
			return fmt.Errorf("%w : la table des montages ne montre pas la destination %s (racine du process déplacée, ou destination sous un lien) : tar refusé", ErrForbiddenPath, d)
		}
	}
	return nil
}

// PseudoMountUnder: a dangerous mount on, under or above dir (spec §5.4).
func PseudoMountUnder(mountinfo, dir string) (mountPoint, fsType string, found bool) {
	dir = CleanPath(dir)
	return dangerousMount(mountinfo, func(mp string) bool {
		return mp == dir || isUnder(mp, dir) || isUnder(dir, mp)
	})
}

// PseudoMountInMounts: a dangerous mount on or under the destination of one of
// the container's Docker mounts. dockerd mounts those again to build any tar of
// the container, so such a mount refuses every directory (review R2).
func PseudoMountInMounts(mountinfo string, mounts []dockerx.Mount) (mountPoint, fsType, destination string, found bool) {
	for _, m := range mounts {
		dst := CleanPath(m.Destination)
		if mp, typ, ok := dangerousMount(mountinfo, func(mp string) bool { return mp == dst || isUnder(mp, dst) }); ok {
			return mp, typ, dst, true
		}
	}
	return "", "", "", false
}

func isUnder(p, parent string) bool {
	return parent == "/" || strings.HasPrefix(p, parent+"/")
}

// unescapeMountinfo decodes the octal escapes of mountinfo (\040 space,
// \011 tab, \012 newline, \134 backslash). A backslash not followed by three
// octal digits is invalid: ok is false.
func unescapeMountinfo(s string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		if i+4 > len(s) {
			return "", false
		}
		n, err := strconv.ParseUint(s[i+1:i+4], 8, 8)
		if err != nil {
			return "", false
		}
		b.WriteByte(byte(n))
		i += 3
	}
	return b.String(), true
}

// MountIdent is what a container path resolves to: the mount it is on
// (statx STATX_MNT_ID, mountinfo field 1), its device and inode.
type MountIdent struct{ MountID, Dev, Ino uint64 }

// LocateMounts returns a copy of mounts where each destination the mount
// table does not show verbatim is replaced by the mount point P of the very
// same mount, when all of this holds; otherwise it stays, and
// CheckVisibleMounts refuses it (fail closed):
//   - the destination, resolved in the container (ident: pathrs, links
//     followed, never out of the root), is on mount n; P resolves to mount
//     n with the same device and inode;
//   - review I4: mount n is this Docker mount (sameMount), so links planted
//     after a chroot cannot map it onto another mount;
//   - P is not "/", and no other Docker mount shows, or is mapped to, P.
func LocateMounts(mountinfo string, mounts []dockerx.Mount, ident func(p string) (MountIdent, error)) []dockerx.Mount {
	lines := map[uint64]mountLine{}
	points := map[string]bool{}
	if _, _, bad := scanMountinfo(mountinfo, func(l mountLine) bool {
		lines[l.ID], points[l.Point] = l, true
		return false
	}); bad {
		return mounts
	}
	taken := map[string]bool{}
	for _, m := range mounts {
		if d := CleanPath(m.Destination); points[d] {
			taken[d] = true
		}
	}
	out := slices.Clone(mounts)
	for i, m := range out {
		dst := CleanPath(m.Destination)
		if points[dst] {
			continue
		}
		want, err := ident(dst)
		if err != nil {
			continue
		}
		l, ok := lines[want.MountID]
		if !ok || l.Point == "/" || taken[l.Point] || !sameMount(m, l) {
			continue
		}
		if got, err := ident(l.Point); err != nil || got != want {
			continue
		}
		out[i].Destination, taken[l.Point] = l.Point, true
	}
	return out
}

// sameMount ties a mountinfo line to a Docker mount: its root (field 4) ends
// the Source of a bind, or is the data directory of a named volume. A tmpfs
// (spec §13: a --tmpfs under a link) has no source to tie: the line must be
// a whole tmpfs (type tmpfs, root "/") outside the mounts the runtime adds
// (/dev, /dev/shm, /proc, /sys), which a link could otherwise borrow.
// Anything else (a bind of a host mount root shown as "/") cannot be tied:
// false.
func sameMount(m dockerx.Mount, l mountLine) bool {
	switch m.Type {
	case "bind":
		return l.Root != "/" && strings.HasSuffix(CleanPath(m.Source), l.Root)
	case "volume":
		return m.Name != "" && strings.HasSuffix(l.Root, "/"+m.Name+"/_data")
	case "tmpfs":
		return l.FSType == "tmpfs" && l.Root == "/" && !runtimeMount(l.Point)
	}
	return false
}

// runtimeMount: on or under /dev, /proc or /sys, which the runtime mounts in
// every container (Docker never writes there either).
func runtimeMount(p string) bool {
	for _, d := range []string{"/dev", "/proc", "/sys"} {
		if p == d || strings.HasPrefix(p, d+"/") {
			return true
		}
	}
	return false
}

// withTmpfs: the Docker mounts plus the --tmpfs destinations
// (HostConfig.Tmpfs, absent from Inspect.Mounts), as tmpfs mounts.
func withTmpfs(mounts []dockerx.Mount, tmpfs []string) []dockerx.Mount {
	out := slices.Clone(mounts)
	for _, d := range tmpfs {
		out = append(out, dockerx.Mount{Type: "tmpfs", Destination: d})
	}
	return out
}

// tmpfsPoints: the cleaned destinations of the tmpfs mounts of ms.
func tmpfsPoints(ms []dockerx.Mount) []string {
	var out []string
	for _, m := range ms {
		if m.Type == "tmpfs" {
			out = append(out, CleanPath(m.Destination))
		}
	}
	return out
}
