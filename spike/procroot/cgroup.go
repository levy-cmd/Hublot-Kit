package procroot

import (
	"errors"
	"strings"
)

// ErrChanged means the process no longer is (or never was) the container's main process.
var ErrChanged = errors.New("container changed")

// ErrNotRegular means the target is not a regular file (device, FIFO, socket...).
var ErrNotRegular = errors.New("not a regular file")

// CgroupHasID reports whether a /proc/<pid>/cgroup content belongs to the container
// with this full 64-character ID. The ID must be a whole path component, either
// "docker-<id>.scope" or "<id>" directly under "docker".
func CgroupHasID(cgroup, id string) bool {
	if len(id) != 64 {
		return false
	}
	for _, line := range strings.Split(cgroup, "\n") {
		path := line[strings.LastIndex(line, ":")+1:]
		parts := strings.Split(path, "/")
		for i, c := range parts {
			if c == "docker-"+id+".scope" || (c == id && i > 0 && parts[i-1] == "docker") {
				return true
			}
		}
	}
	return false
}
