//go:build linux

package fsview

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// Local constants, absent from x/sys (spec §5.1 step 9).
const (
	configfsMagic     = 0x62656570 // include/uapi/linux/magic.h
	fuseCtlSuperMagic = 0x65735543 // fs/fuse/control.c
)

// pseudoFS lists the 15 filesystems whose files are read with the agent's host
// privileges (e.g. /proc/kallsyms gives the real kernel addresses).
var pseudoFS = map[uint32]string{
	unix.PROC_SUPER_MAGIC:    "proc",
	unix.SYSFS_MAGIC:         "sysfs",
	unix.CGROUP_SUPER_MAGIC:  "cgroup",
	unix.CGROUP2_SUPER_MAGIC: "cgroup2",
	unix.DEBUGFS_MAGIC:       "debugfs",
	unix.TRACEFS_MAGIC:       "tracefs",
	unix.SECURITYFS_MAGIC:    "securityfs",
	unix.BPF_FS_MAGIC:        "bpf",
	unix.PSTOREFS_MAGIC:      "pstore",
	unix.EFIVARFS_MAGIC:      "efivarfs",
	unix.BINFMTFS_MAGIC:      "binfmt_misc",
	unix.SELINUX_MAGIC:       "selinuxfs",
	unix.SMACK_MAGIC:         "smackfs",
	configfsMagic:            "configfs",
	fuseCtlSuperMagic:        "fusectl",
}

// checkNotPseudo refuses a descriptor (O_PATH is enough) that lives on a
// pseudo filesystem. It checks the resolved descriptor, never the path text,
// so a link that leads there is refused too.
func checkNotPseudo(fd int, path string) error {
	var st unix.Statfs_t
	if err := unix.Fstatfs(fd, &st); err != nil {
		return fmt.Errorf("fstatfs %s : %w", path, err)
	}
	if name, bad := pseudoFS[uint32(st.Type)]; bad {
		return fmt.Errorf("%w : %s est sur %s", ErrForbiddenPath, path, name)
	}
	return nil
}
