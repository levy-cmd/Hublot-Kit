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
- 5 failed logins per minute and per IP address, then `rate_limited`. Every failure is written to the audit log.
- The session lives in memory for 12 hours. Its cookie is `HttpOnly`, `SameSite=Strict`, and `Secure` with TLS or an
  `https` public origin.
- Every request other than GET or HEAD must carry an `Origin` header equal to the trusted origin (`--public-origin`,
  or the `Host` of the request). `X-Forwarded-*` headers are never trusted.
- DNS rebinding: without `--public-origin`, the trusted origin comes from `Host`, so a rebound name passes the
  `Origin` check; the `SameSite=Strict` cookie and the token are then what protect you, and a rebound page can use
  up the login attempts of `127.0.0.1` for a minute. Exposed beyond `127.0.0.1`, set `--public-origin`.

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
  are shown, if the masking above finds nothing in them. Other labels show their name, never their value, and cannot
  be revealed: a Traefik `basicauth.users` label holds a password hash the rules above do not recognise.
- A masked value is revealed one at a time, and each reveal is written to the audit log (the name, never the value)
  before the value is sent.
- This is a heuristic: [some forms are not recognised](limitations.md#secret-masking). Logs are never masked.

## Audit log

`audit.log` in the data directory, one JSON line per event: failed logins, revealed variables, edited and uploaded
files. A failed login also records the IP address of the TCP connection (never `X-Forwarded-For`). It is append-only and is not rotated in v0.1.

## Docker socket

Mounting the socket `:ro` protects nothing: it is a socket, not a file. A Docker socket proxy that only allows the
endpoints Hublot-Kit uses (`_ping`; containers: list, inspect, changes, logs, archive; volumes: inspect) narrows what the
agent can ask Docker, but with `--pid=host` the agent stays root on the host.

## Reporting a vulnerability

Please use GitHub's private vulnerability reporting (the **Security** tab of the repository, **Report a
vulnerability**) rather than a public issue.
