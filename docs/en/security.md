# Security model

## Hublot-Kit is root on the host

The agent talks to the Docker socket, which is root access to the host, and with `--pid=host` it reads any
container's files through `/proc/<pid>/root`. Whoever can log in to Hublot-Kit, or take over the agent, is root on
the server. The measures below limit mistakes and exposure; they do not protect against a compromised agent.

Treat the access token like the root password of the server.

## Exposure

- The agent listens on `127.0.0.1:7878` by default. On any other address without TLS it prints
  `WARNING: listening on … without TLS` at startup (legitimate inside a container published on `127.0.0.1`, or behind a reverse proxy).
- Reach it through an SSH tunnel, or through an HTTPS reverse proxy with `--public-origin` set
  ([installation](install.md#behind-an-https-reverse-proxy)).
- The agent makes no outbound connection: no telemetry, no update check.

## Login and session

- The token is 32 random bytes, printed once. Only its SHA-256 hash is stored (`token.sha256`), and it is compared in
  constant time. `hublot reset-token` replaces it.
- 5 failed logins per minute and per IP address, then `rate_limited`. Each attempt is counted before its token is
  read, so a burst of requests gets no more. Failures are written to the audit log, at most 10 lines a minute per
  address (an IPv6 address counts for its whole /64); past that, one `suppressed` line for that address, and a minute
  later one line per address counts the failures that were not written, even if they stopped.
- A connection must send its first byte within 5 seconds, a request must be read within 15 seconds (headers within
  10), an edit or upload once logged in within 10 minutes; an idle connection is closed after a minute. The agent
  keeps at most 512 connections open, 64 per address (`--max-conns-per-ip`). A download, the preview of a file
  (of a container or an image) or a logs stream whose client stops reading for 30 seconds is ended.
- The session lives in memory for 12 hours. Its cookie is `HttpOnly`, `SameSite=Strict`, and `Secure` with TLS or an
  `https` public origin.
- Every request other than GET or HEAD must carry an `Origin` header equal to the trusted origin (`--public-origin`,
  or the `Host` of the request). `X-Forwarded-*` headers are never trusted.
- DNS rebinding: without `--public-origin`, the trusted origin comes from `Host`, so a rebound name passes the
  `Origin` check; the `SameSite=Strict` cookie and the token are then what protect you, and a rebound page can use
  up the login attempts of `127.0.0.1` for a minute, or hold its 64 connections. Exposed beyond `127.0.0.1`, set `--public-origin`.

## Content of the containers

- Files, logs, file names and variables are always escaped by the interface; `{@html}` is forbidden in the code and
  enforced by a lint rule. The preview only serves `text/plain` or a PNG, JPEG, GIF or WebP image, with `nosniff`;
  SVG and HTML are shown as text, never rendered. Downloads are always `attachment`.
- Headers: a strict `Content-Security-Policy` (scripts limited to the agent and the hashes of the embedded
  `index.html`, `frame-ancestors 'none'`), `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`.

## Reading files

- The agent pins the container's process with a pidfd, checks its cgroup, and opens every path with
  `filepath-securejoin` inside the container's root: a symbolic link can never lead outside the container.
- Pseudo-filesystems (`/proc`, `/sys` and the like) are listed but cannot be opened: they would be read with the
  agent's host privileges. Device nodes, FIFOs and sockets are refused: a container's root could create a node of a
  host device.
- A container that bind-mounts the host (`-v /:/host`) shows the host in the explorer, and its files can be read:
  the user is root anyway. Downloading a folder of such a container is refused.

## Writing files

- `--read-only` disables every write, in a single place in the code.
- Every write goes through a confirmation that names the container and the path, then through Docker
  (`PUT /archive`), never directly. The path must contain no symbolic link at all.
- Writes are refused in a container that bind-mounts `/`, `/proc`, `/sys` (or below) or `/dev` of the host, and
  where Docker does not write what the container sees (`/etc/hosts`, tmpfs, mount points).
- The audit log records the intent before the write and the result after it. If the intent cannot be written,
  nothing is written in the container.

## Secrets

- The X-ray masks a variable when its name contains `PASSWORD`, `PASSWD`, `PASS`, `TOKEN`, `KEY`, `SECRET`,
  `CREDENTIAL`, `PRIVATE` or `DSN`, when its value holds a URL with credentials, or a password in a known form
  (connection strings, `--requirepass …`, `Authorization: Bearer …`). The command and the error message are masked
  part by part.
- Labels follow an allow list: only the values of `com.docker.compose.*` and `org.opencontainers.image.*` labels
  are shown, if the masking above finds nothing in them and they are not paths of the host (Compose's `config_files`,
  `working_dir`, `environment_file`). Other labels show their name, never their value, and cannot
  be revealed: a Traefik `basicauth.users` label holds a password hash the rules above do not recognise.
- A masked value is revealed one at a time, and each reveal is written to the audit log (the name, never the value)
  before the value is sent.
- The Secrets page (`GET /api/secrets`) returns names only: masked variables, arguments by their place and what was detected in them (never their text,
  which a partial mask could let through), keys of the masked labels. It has no reveal of its own: a value goes through the same journaled request as the X-ray, one
  variable at a time, under the `Origin` check. Like every answer of the API, it is not cached
  (`Cache-Control: no-store`), and the interface writes no value to the browser's storage. Its cost is bounded: four
  `docker inspect` at a time for the whole agent, 3 s each, 15 s and 1,000 containers per inventory; past that, a
  container is marked unavailable.
- This is a heuristic: [some forms are not recognised](limitations.md#secret-masking). Logs are never masked.

## Snapshots

- A snapshot keeps no file content: metadata and, for each regular file, an HMAC-SHA256 of its content, with a key of
  the agent (`snapshot.key`, 32 bytes, 0600, in the data directory). A bare SHA-256 of a small secret file (a code, a
  short password) could be guessed offline; without the key, it cannot. Neither the fingerprints nor the key id leave
  the agent: no API response contains one.
- The key is created atomically (temporary file, `fsync`, `linkat`) and read back without following a link; it is
  refused if it is not a regular file of the agent's user, if group or others have any right on it, or if it is not
  exactly 32 bytes. `hublot rotate-snapshot-key` (agent stopped) writes a new one: the snapshots taken with the old
  one then compare by their metadata only. Deleting or changing the key has the same effect.
- The manifests live in `snapshots/` (0700), opened without following a link and checked through its descriptor;
  each file is written there in 0600, relative to that descriptor, published by `renameat`. **The names of the
  container's files are written there**: a name may be sensitive; it is already visible in the explorer, and the
  directory is root's. The key and the manifests are in the same directory: whoever reads it has both, but they are
  root on the host. The key protects what leaves the directory in pieces (a manifest pasted in a ticket, a partial
  backup).
- The walk of a running container opens every path from the pinned root without following any link (`openat2` with
  `RESOLVE_NO_SYMLINKS`): a link planted during the walk is never followed, the file is noted unreadable. It reads no
  pseudo filesystem, no FIFO, no device, crosses no mount point, and reads with `O_NOATIME` (the access times of the
  production disk do not change). The export of a stopped container goes through the same refusals as the explorer
  (a bind of the host's `/`, `/proc`, `/sys` or `/dev`, judged as written and as resolved on the host); its content
  is hashed only under the caps, never for a sparse entry.

## Audit log

`audit.log` in the data directory, one JSON line per event: failed logins, revealed variables, edited and uploaded
files, snapshots taken and deleted. A failed login also records the IP address of the TCP connection (never `X-Forwarded-For`). It is append-only and is not rotated in v0.1.

## Docker socket

Mounting the socket `:ro` protects nothing: it is a socket, not a file. A Docker socket proxy that only allows the
endpoints Hublot-Kit uses (`_ping`; containers: list, inspect, changes, logs, archive; volumes: inspect) narrows what the
agent can ask Docker, but with `--pid=host` the agent stays root on the host.

## Reporting a vulnerability

Please use GitHub's private vulnerability reporting (the **Security** tab of the repository, **Report a
vulnerability**) rather than a public issue.
