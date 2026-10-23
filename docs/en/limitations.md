# Known limitations (v0.1)

v0.1 is a first public version. This page lists what it does not do, and the cases it knowingly does not detect.
The technical spec ([§13, in French](../superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md)) has every detail.

## Scope

- One Docker server, one user. Multiple servers or users: later.
- Not supported: rootless Docker, `userns-remap`, Podman, Kubernetes.
- Not in v0.1: processes and resources, start/stop/restart, web terminal (planned for v0.2); certificates from
  Let's Encrypt (bring your own); rotation of the audit log.

## Host

- Kernel 5.6 or later to read; 5.8 or later to edit or upload in a running container. Docker API 1.44 or later,
  overlay2 or containerd storage.
- Only the cgroup layouts of Docker are recognised: with the cgroupfs driver and a custom `--cgroup-parent`, a
  running container answers `container_changed` and cannot be explored. With the systemd driver it works.
- Measured on Ubuntu (AppArmor) and Fedora (SELinux enforcing). Other security modules were not measured.

## Files

- Pseudo-filesystems (`/proc`, `/sys`…) appear as folders that cannot be opened. A pseudo-filesystem type that
  the agent does not know would not be refused; device nodes stay refused in any case.
- A stopped container is read from an index of its full archive, built on first access: this can take seconds on a
  large image, with a progress bar. The index is capped at 4 GiB read, 5 minutes and 1,000,000 entries
  (`too_large`); three indexes are kept in memory.
- While Docker exports a stopped container (indexing, or download of a folder), it answers nothing else about that
  container. During the indexing, browsing answers at once with the progress; the other requests on that container
  (X-ray, logs, writes, browsing during a download) wait for the end of the export, and fail after 10 seconds
  (`docker_unavailable`) if it lasts longer.
- A stopped container that bind-mounts `/`, `/proc`, `/sys` (or below) or `/dev` of the host cannot be explored (the
  error names the mount): its archive would contain the host. Its logs, changes and X-ray stay available.
- A host folder bind-mounted into a stopped container, with a `/proc`, `/sys`, FUSE or NFS mount below it, is not
  detected: indexing may then make Docker read it, and a stuck FUSE or NFS server can block `dockerd`.
- A FUSE mount owned by a running container that stops answering: requests to that container fail after 10
  seconds, at most 4 at a time, and the stuck reads stay blocked inside the agent until it restarts.
- File names that are not valid UTF-8 are listed, but cannot be opened from the interface.
- Downloading a folder of a running container is refused when the archive would cross a host bind, a
  pseudo-filesystem or a FUSE mount, and always for `/`. A mount added between the check and the download (it needs
  `CAP_SYS_ADMIN` in the container) or a "hard" NFS mount below the folder is not refused, and can block `dockerd`.
- A mount added later on the host below the source of a bind does not appear in the running container (default
  `rprivate` propagation) and is not detected, although Docker may see it when it builds the archive.
- A container opened while stopped, then started before the download: only the check of the host binds is run again,
  not the other mount checks of a running container.
- A container whose process root was moved by a chroot: the explorer shows the view from the root of its PID 1, and
  folder downloads are refused when the mount table lacks `/` or a mount that Docker declares. A mount destination
  visible as is, with no symbolic link on its path, is taken without proof of identity: a download may then block
  `dockerd`.

## Writes

- Not atomic: Docker deletes the file and creates a new one. Hard links to the old file are broken, a process that
  keeps it open still sees the old content, and a write cut after one minute leaves the file lost or truncated.
- Between the checks and the write, a process of the container can still change the file or put a symbolic link in
  the path; Docker then resolves the path inside the container. It can reach another mount of the container, such
  as a bind of a host folder other than `/`, `/proc`, `/sys` or `/dev`.
- Refused where Docker does not write what the container sees: `/etc/hosts`, `/etc/hostname`, `/etc/resolv.conf`,
  mount points, tmpfs, `/dev`, `/proc`, `/sys`. A tmpfs declared under another tmpfs, a volume or a bind makes every
  write to that running container refused (fail closed).
- Mounts that `docker inspect` does not show (Swarm configs and secrets, `--init`, CDI or `--gpus`): an edit in a
  running container is refused, but an upload, or any write to a stopped container, is not checked.
- A write under a stuck FUSE or "hard" NFS mount can block `dockerd`, and the container stays locked for writes until
  the agent restarts; the interface may then wrongly say that the write may have happened.
- A chroot jail mounted by the container itself (`CAP_SYS_ADMIN`) is not seen.
- A decoy tmpfs mounted by the container itself (`CAP_SYS_ADMIN`) on the target of a symbolic link placed after the
  start: a write can then succeed without the container seeing it (not an escape).
- Stopped container whose tmpfs destination goes through a symbolic link located in a volume or a bind: a link
  placed there later (by the host or another container) is not seen by the index, and the tmpfs may be mounted
  elsewhere at the next start; a write can then succeed without the restarted container seeing it (not an escape).

## Logs

- Logs are shown as `docker logs` shows them: secrets written by the application are not masked.
- Beyond 1,000 lines per second, extra lines are not displayed but counted; the download has them all.
- At most 8 streams for the agent; when they are all taken, the oldest stream gives its place and says so.
- A client gone without closing its connection (sleep, network change) keeps its stream open on the agent side for
  about 15 minutes, and holds one of the 8 slots until then, unless a newer stream takes it.
- Resuming after a cut can replay a few lines that share a timestamp, count dropped lines twice, or skip lines after
  a log rotation or a clock going backwards (or mark them as cut, when the clock goes back more than a second).
- A line longer than 16 KiB that was still being written when the stream was cut, and begun more than a second before
  the cut, comes back without its start: the live view loses that start, but marks the line as cut instead of showing
  it as complete. The download has the whole line.
- When the view opens, the past lines shown may start with the end of a line longer than 16 KiB, without any mark:
  Docker counts the parts of a long line, not lines, and gives no way to tell. The download has the whole line.
- A logging driver Docker cannot read back (`none`, `syslog`…) gives `logs_unavailable`.

## Search (v0.2)

- Files can be searched only in the index already built of a stopped container the explorer has opened. A running container has no index: open the explorer.
- The query of a file search goes in the body of a POST (`…/fs/search`), never in an address: the agent neither logs nor keeps it, and an ordinary access log of a reverse proxy does not see it. A proxy set to log request bodies would see it: a file name can be sensitive.
- While a stopped container is exported (its index being built), Docker locks the container. The agent recognises that build by the container's full ID or name, never by a short ID: a direct API call with a short ID then waits for the lock, up to the route's deadline (10 s for files and search; the key figures wait for that lock whatever the ID, up to 30 s). The interface always sends the full ID.

## Secret masking

The X-ray masks secrets by name and by known forms. Not recognised in v0.1, among others:

- JSON values (`{"token": "…"}`), `user:password@host` without `://`, `password = …` with spaces, a variable named
  `DB_PWD`;
- in the command: `mysql -pSECRET`, secrets in a script or in a file passed as an argument, `--password -s3cr3t`,
  `--pwd s3cr3t` as two arguments, keys with non-ASCII or invisible characters, `Authorization: Token …` or
  `ApiKey …`;
- some masks leave a remainder visible, and `env PWD=/srv` is masked although it is not a secret.

The command itself cannot be revealed in v0.1 (`docker inspect` can).
