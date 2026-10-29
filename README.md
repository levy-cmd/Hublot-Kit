# Hublot-Kit

> **Security: installing Hublot-Kit gives it root on the host.**
> It talks to the Docker socket and reads the files of your containers. Keep it on `127.0.0.1` and reach it
> through an SSH tunnel or an HTTPS reverse proxy. Read [the security model](docs/en/security.md) first.

[Lire en français](README.fr.md)

Explore the inside of the Docker containers of a server from a browser, without a terminal. Pick a container and see
its files, what changed since its image, its logs and its configuration; browse the files of an image; take a photo
of a container's files and compare it later.

![Container list, dark theme](docs/images/containers.png)
![File explorer with the changes since the image](docs/images/explorer.png)

## What it does (v0.2)

- **Containers**: name, image, Compose project, status, ports, uptime; tabs, filters, key figures (files, image
  size, container size) and labels, filtered.
- **Files**: the whole tree of a running or a stopped container, text and image preview, size, owner, permissions.
- **Download** a file, or a folder as a tar archive.
- **Edit** a text file or **upload** a file, after a confirmation that names the container and the path.
- **Diff with the image**: what was added, modified or deleted since the image, and the line diff of a modified
  file between its version in the image and in the container.
- **Logs**: live stream, search, pause and resume, download.
- **Overview** of a container: image, state, command, ports, and under **Details** its networks and mounts; its
  **Secrets** tab holds the environment (secrets masked, revealed one at a time and logged).
- **Images**: every image of the daemon, its layers and its configuration (masked), and its files read only,
  without creating a container.
- **Snapshots**: a photo of a container's files (metadata and a keyed fingerprint, never the content), compared with
  another photo or with the current container.
- **Secrets**: the names of everything the agent masks, across every container; never a value unless revealed, one
  at a time.
- **Search** (`Ctrl K`): containers, Compose projects, pages, and the files of an index already built.
- **Themes**: dark by default, light on demand, kept in the browser.
- **Read-only mode** (`--read-only`): no write into the containers; the interface shows the agent's real state.

One server, one user, an interface in English and French, a single static binary with the interface embedded, no
database, no telemetry. Processes and resources, start/stop and a web terminal are postponed after v0.2.

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
  ghcr.io/levy-cmd/hublot-kit:0.2
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

To try it on throwaway containers first (a service without a shell, a stopped one, an edited nginx, fake secrets),
see [the demo](examples/demo/).

## Documentation

- [A tour of the interface](docs/en/guide.md): the layout, the search, the panels, the Images, Secrets and Snapshots
  pages, the theme, read-only mode as the interface shows it.
- [Installation](docs/en/install.md): Docker, binary and systemd, reverse proxy, token, upgrade, uninstall, verifying
  the signatures and SBOMs of a release.
- [Security model](docs/en/security.md): what the agent can do, what protects it, how to expose it.
- [Known limitations](docs/en/limitations.md): what Hublot-Kit does not do, or does not detect.

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
`127.0.0.1:7878`: `cd web && npm run dev`. The technical specs are in docs/superpowers/specs (in French):
[v0.1](docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md), the reference, and
[v0.2](docs/superpowers/specs/2026-10-23-hublot-kit-v0.2-design.md), its complement.

## How this project is built

Hublot-Kit is developed with AI coding agents, under a strict process. Every change follows a written plan for its
phase ([docs/superpowers/plans](docs/superpowers/plans/)). The escape tests, which check that a path cannot lead out
of a container, are written before the code they protect. Each task gets two independent reviews, and the whole
branch is reviewed again before each release. Each release is signed, then verified from another machine without
credentials, by following the installation guide as written.

## License

[MIT](LICENSE)
