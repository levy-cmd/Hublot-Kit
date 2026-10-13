# Installing Hublot-Kit

> **Hublot-Kit is root on the host in practice.** Read [the security model](security.md) before installing, and
> [the known limitations](limitations.md) before relying on it.

## Requirements

- Linux amd64 or arm64.
- Kernel 5.6 or later to read files; 5.8 or later to edit or upload files in a running container.
- Docker Engine with API 1.44 or later (`docker version` shows it), overlay2 or containerd storage.
- Not supported in v0.1: rootless Docker, `userns-remap`, Podman, Kubernetes.

## Docker (recommended)

The same command works on AppArmor hosts (Ubuntu, Debian) and SELinux hosts (Fedora, RHEL). It was measured on
Ubuntu with AppArmor and on Fedora with SELinux enforcing.

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

`0.1` follows the 0.1.x patch releases; `0.1.0` never moves. There is no `latest` tag.

### Why each option

| Option | Why |
|---|---|
| `--pid=host` | to see `/proc/<pid>` of the containers, which is how the agent reads their files. |
| `--cgroupns=host` | required: the agent proves that a process belongs to a container through its cgroup path. In a private cgroup namespace it cannot, and every file request answers `container_changed`; the agent then prints `WARNING: private cgroup namespace (/proc/self/cgroup = 0::/): …` at startup. |
| `--cap-add SYS_PTRACE` | to read the files of a container that runs as another user, or that holds capabilities the agent lacks. |
| `--security-opt apparmor=unconfined` | AppArmor hosts: without it, containers started with `--privileged` or `apparmor=unconfined` cannot be explored (the error names this option). No effect on other hosts. |
| `--security-opt label=disable` | SELinux hosts: lets the agent read other containers and use the Docker socket. Docker already applies it to a container started with `--pid=host`, but that is not a documented contract. Do not use any other `label=` option: it would cut the agent off the socket. No effect on other hosts. |
| `-v /var/run/docker.sock:…` | the agent's only way to Docker. Mounting it `:ro` protects nothing. |
| `-v hublot-data:/data` | the hash of the access token and the audit log. |
| `-p 127.0.0.1:7878:7878` | reachable from the server only; use an SSH tunnel or a reverse proxy. |

`--privileged` is not needed. The agent is already root on the host in practice: lifting its AppArmor confinement,
or running it as `spc_t` under SELinux, changes which containers it can read, not what it can do.

### First login

The access token is printed once, on the first start, on the line `Access token (shown only once): …`.

```sh
docker logs hublot-kit
```

From your computer, open a tunnel, browse to <http://127.0.0.1:7878> and paste the token.

```sh
ssh -L 7878:127.0.0.1:7878 user@server
```

### Options

Every flag has an environment variable. With Docker, add flags after the image name, or variables with `-e` before
it.

| Flag | Variable | Default |
|---|---|---|
| `--listen` | `HUBLOT_LISTEN` | `127.0.0.1:7878` (`0.0.0.0:7878` in the image) |
| `--data-dir` | `HUBLOT_DATA_DIR` | `/var/lib/hublot-kit` (`/data` in the image) |
| `--read-only` | `HUBLOT_READ_ONLY` | `false`: with `true`, no write at all, and the write buttons disappear |
| `--tls-cert`, `--tls-key` | `HUBLOT_TLS_CERT`, `HUBLOT_TLS_KEY` | empty: serve HTTPS directly with a certificate you provide |
| `--public-origin` | `HUBLOT_PUBLIC_ORIGIN` | empty: the public URL behind a reverse proxy |

`hublot --help` lists them; `hublot --version` prints the version.

## Behind an HTTPS reverse proxy

Keep the port on `127.0.0.1`, point the proxy at `http://127.0.0.1:7878`, and give the agent its public origin, for
example `-e HUBLOT_PUBLIC_ORIGIN=https://hublot.example.com` before the image name. Without it, a proxy that changes
the scheme makes every request other than GET fail with `forbidden_origin`, login included, and the session cookie
is not marked `Secure`.

The logs tab is a Server-Sent Events stream: the proxy must not buffer it (the agent sends `X-Accel-Buffering: no`,
which nginx honours) and must allow long reads (the stream sends a ping every 15 seconds). The agent never trusts
`X-Forwarded-*` headers.

Without a proxy, `--tls-cert` and `--tls-key` serve HTTPS directly. v0.1 does not obtain certificates by itself.

## Binary and systemd

The alternative to the container: the static binary, run as root by systemd. Check the archive first
([Verify the release](#verify-the-release)). Use `amd64` or `arm64`.

```sh
V=0.1.0
ARCH=amd64
curl -fLO "https://github.com/levy-cmd/Hublot-Kit/releases/download/v$V/hublot-kit_${V}_linux_${ARCH}.tar.gz"
tar -xzf "hublot-kit_${V}_linux_${ARCH}.tar.gz"
sudo install -m 0755 hublot /usr/local/bin/hublot
sudo install -m 0644 hublot-kit.service /etc/systemd/system/hublot-kit.service
sudo /usr/local/bin/hublot reset-token
sudo systemctl daemon-reload
sudo systemctl enable --now hublot-kit
```

`hublot reset-token` before the first start prints the token on your terminal (`New access token (shown only once): …`). Started first, the agent would print
it in the system journal, which members of the `adm` or `systemd-journal` groups can read.

On SELinux hosts, `install` gives the binary the right label; a binary moved with `mv` from a home directory keeps a
label that systemd refuses to run (`sudo restorecon /usr/local/bin/hublot` fixes it).

The unit listens on `127.0.0.1:7878`, keeps its data in `/var/lib/hublot-kit` (mode 0700) and runs with
`NoNewPrivileges`, `ProtectSystem=strict` and `PrivateTmp`. It does not hide `/proc` or drop capabilities: the
explorer needs them.

### Settings

Change options with a drop-in, never by editing the unit:

```sh
sudo systemctl edit hublot-kit
```

```ini
[Service]
Environment=HUBLOT_PUBLIC_ORIGIN=https://hublot.example.com
Environment=HUBLOT_READ_ONLY=true
```

```sh
sudo systemctl restart hublot-kit
```

## Access token

A new token invalidates the old one once the agent restarts; until then, a running agent still accepts the old one.
After 5 failed logins in a minute from one address, logins from that address are refused for the rest of the minute.

With Docker:

```sh
docker exec hublot-kit /hublot reset-token
docker restart hublot-kit
```

With systemd:

```sh
sudo /usr/local/bin/hublot reset-token
sudo systemctl restart hublot-kit
```

## Data directory

`--data-dir` must be dedicated to Hublot-Kit. It holds `token.sha256` (the hash of the token, never the token) and
`audit.log`. Missing, it is created with mode 0700. Existing, its mode is kept, but the agent refuses to start if it
is a symbolic link, if it belongs to another user, or if its group or others can write to it (`/tmp`, for example).
Its files are never opened through a symbolic link. The audit log is not rotated in v0.1.

## Upgrade

With Docker, pull the image, remove the container and run the command of [Docker](#docker-recommended) again; the
`hublot-data` volume keeps the token.

```sh
docker pull ghcr.io/levy-cmd/hublot-kit:0.1
docker rm -f hublot-kit
```

With systemd, download and verify the new archive, then replace the binary and restart:

```sh
sudo install -m 0755 hublot /usr/local/bin/hublot
sudo systemctl restart hublot-kit
```

## Uninstall

With Docker:

```sh
docker rm -f hublot-kit
docker volume rm hublot-data
```

With systemd:

```sh
sudo systemctl disable --now hublot-kit
sudo rm -rf /usr/local/bin/hublot /etc/systemd/system/hublot-kit.service /etc/systemd/system/hublot-kit.service.d
sudo systemctl daemon-reload
sudo rm -rf /var/lib/hublot-kit
```

## Verify the release

Releases are built and signed by GitHub Actions, with keyless [cosign](https://docs.sigstore.dev/) signatures tied to
the release workflow of this repository. You need cosign 3.0 or later.

What a valid signature proves: the file or the image was produced by that workflow, run on that tag, and has not
changed since. What it does not prove: that the code of the tag was reviewed, or that an SBOM is complete (it lists
what syft, for the archives, and BuildKit, for the image, found in the binary). No SLSA provenance is published in
v0.1.

### Archives

`checksums.txt` lists the SHA-256 of the archives and of their SPDX SBOMs (`.sbom.json`); its signature covers them.
The `.sigstore.json` bundles are not listed: each one carries its own certificate, checked by `cosign verify-blob`.
Each archive also has its own bundle.

```sh
V=0.1.0
ARCH=amd64
BASE="https://github.com/levy-cmd/Hublot-Kit/releases/download/v$V"
curl -fLO "$BASE/hublot-kit_${V}_linux_${ARCH}.tar.gz"
curl -fLO "$BASE/checksums.txt"
curl -fLO "$BASE/checksums.txt.sigstore.json"
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/levy-cmd/Hublot-Kit/.github/workflows/release.yml@refs/tags/v$V" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum --ignore-missing -c checksums.txt
```

### Image

The image is signed by digest, for linux/amd64 and linux/arm64, and carries an SBOM attestation.

```sh
cosign verify \
  --certificate-identity-regexp '^https://github\.com/levy-cmd/Hublot-Kit/\.github/workflows/release\.yml@refs/tags/v0\.1\.[0-9]+$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/levy-cmd/hublot-kit:0.1
docker buildx imagetools inspect ghcr.io/levy-cmd/hublot-kit:0.1 --format '{{ json .SBOM }}'
```

To run exactly the image you verified, replace `:0.1` in the command of [Docker](#docker-recommended) with the digest
that `cosign verify` prints (`ghcr.io/levy-cmd/hublot-kit@sha256:…`).
