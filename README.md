# Hublot-Kit

> **Security: installing Hublot-Kit gives it root on the host.**
> It talks to the Docker socket and reads the files of your containers. Keep it on `127.0.0.1` and reach it
> through an SSH tunnel or an HTTPS reverse proxy. Read [the security model](docs/en/security.md) first.

[Lire en français](README.fr.md)

Explore the inside of the Docker containers of a server from a browser, without a terminal. Pick a container and see
its files, what changed since it was created, its logs and its configuration.

![Container list](docs/images/containers.png)
![File explorer with the changes since creation](docs/images/explorer.png)

## What it does (v0.1)

- **Containers**: name, image, status, ports, uptime; filter and search.
- **Files**: the whole tree of a running or a stopped container, text and image preview, size, owner, permissions.
- **Download** a file, or a folder as a tar archive.
- **Edit** a text file or **upload** a file, after a confirmation that names the container and the path.
- **Changes**: what was added, modified or deleted since the container was created.
- **Logs**: live stream, search, pause and resume, download.
- **X-ray**: image, state, command, environment (secrets masked, revealed one at a time and logged), ports, networks,
  mounts.
- **Read-only mode** (`--read-only`): no write at all.

One server, one user, an interface in English and French, a single static binary with the interface embedded, no
database, no telemetry. Processes and resources, start/stop and a web terminal are planned for v0.2.

## Quick start

Requirements: Linux amd64 or arm64, kernel 5.6 or later (5.8 to edit files of a running container), Docker Engine
with API 1.44 or later, overlay2 or containerd storage. The same command works on AppArmor (Ubuntu, Debian) and
SELinux (Fedora, RHEL) hosts; [why each option is needed](docs/en/install.md#why-each-option).

```sh
docker run -d --name hublot-kit \
  --pid=host --cgroupns=host --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined \
  --security-opt label=disable \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v hublot-data:/data \
  -p 127.0.0.1:7878:7878 \
  ghcr.io/levy-cmd/hublot-kit:0.1
```

The access token is printed once, on the first start, on the line `Access token (shown only once): …`.

```sh
docker logs hublot-kit
```

From your computer, open a tunnel, then browse to <http://127.0.0.1:7878> and paste the token.

```sh
ssh -L 7878:127.0.0.1:7878 user@server
```

Prefer a plain binary run by systemd? See [Binary and systemd](docs/en/install.md#binary-and-systemd).

## Documentation

- [Installation](docs/en/install.md): Docker, binary and systemd, reverse proxy, token, upgrade, uninstall, verifying
  the signatures and SBOMs of a release.
- [Security model](docs/en/security.md): what the agent can do, what protects it, how to expose it.
- [Known limitations](docs/en/limitations.md): what v0.1 does not do, or does not detect.

## Development

Requirements: Go 1.27 or later, Node 24 (22.17 or later), Docker. The Go code that touches `/proc` only runs on Linux:
`make test-linux` runs it as root in a `golang` container.

```sh
make test
make build
make test-linux
```

On macOS (Docker Desktop), `/proc/<pid>/root` only exists inside Docker's virtual machine, so the agent runs in a
container: `make web && scripts/dev-agent.sh`. The interface in development mode proxies to the agent on
`127.0.0.1:7878`: `cd web && npm run dev`. The technical spec is in
[docs/superpowers/specs](docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md) (in French).

## License

[MIT](LICENSE)
