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
	return slices.Contains(pseudoFSNames, t) || t == "fuse" || t == "fuseblk" || strings.HasPrefix(t, "fuse.")
}

// dangerousMount returns the first mount of /proc/<pid>/mountinfo (mount
// points relative to the process's root) of a dangerous type whose mount point
// matches. Every line is read, stacked mounts included. A line that cannot be
// parsed, or with an invalid escape, counts as dangerous: fail closed.
func dangerousMount(mountinfo string, match func(mountPoint string) bool) (mountPoint, fsType string, found bool) {
	for _, line := range strings.Split(mountinfo, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// id parent major:minor root mount-point options [optional fields…] - type source super-options
		f := strings.Fields(line)
		sep := slices.Index(f, "-")
		if len(f) < 5 || sep < 5 || sep+1 >= len(f) {
			return line, "ligne de mountinfo illisible", true
		}
		mp, ok := unescapeMountinfo(f[4])
		if !ok {
			return f[4], "échappement de mountinfo invalide", true
		}
		if typ := f[sep+1]; dangerousFSType(typ) && match(mp) {
			return mp, typ, true
		}
	}
	return "", "", false
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
