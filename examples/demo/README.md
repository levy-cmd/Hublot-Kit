# Hublot-Kit demo

Four throwaway containers to explore, and the agent itself, in one Compose file. It is meant for the 90-second video
and the screenshots of the README.

> **The agent gets root on the Docker host and shows every container of that host**, not only the demo ones: the
> same rights as the [documented command](../../docs/en/install.md#why-each-option). Run the demo on a machine or a
> virtual machine you can trust it with, and with no other container if you record it.

Requirements: Docker Engine or Docker Desktop with Compose v2, `curl`, the port `127.0.0.1:7878` and the container
names `api`, `worker`, `web`, `db` and `hublot-demo` free.

```sh
./demo.sh up
docker logs hublot-demo
```

Browse to <http://127.0.0.1:7878> and paste the token of the line `Access token (shown only once): …`.
`./demo.sh down` removes the containers, the network and the volumes the Compose file created; the images stay
(`traefik/whoami:v1.11.0`, `alpine:3.20.10`, `nginx:1.27.5-alpine`, `postgres:17.11-alpine`,
`ghcr.io/levy-cmd/hublot-kit:0.2`).

## What each service shows

| Service       | Image                             | Shows                                                                                   |
|---------------|-----------------------------------|-----------------------------------------------------------------------------------------|
| `api`         | `traefik/whoami:v1.11.0`          | A running service with no shell: `docker exec -it api sh` fails. Config in `/etc/api`. |
| `worker`      | `alpine:3.20.10`                  | Stopped by `demo.sh up` after about six jobs: files in `/var/lib/worker/out`, and its logs. |
| `web`         | `nginx:1.27.5-alpine`             | `/etc/nginx/nginx.conf` edited by `demo.sh up`: `worker_connections`, `gzip`.          |
| `db`          | `postgres:17.11-alpine`           | Fake credentials (`demo-password-not-secret`, `API_TOKEN`, `DATABASE_URL`…) to mask.    |
| `hublot-demo` | `ghcr.io/levy-cmd/hublot-kit:0.2` | The agent, with the options of the documented `docker run` command, on `127.0.0.1:7878`. |

## The demo in 90 seconds

1. **The problem.** In a terminal, `docker exec -it api sh` answers `executable file not found`: the image has no
   shell, so there is no way to look inside.
2. **A container without a shell.** In Hublot-Kit, open `api`, then its files: `/etc/api/config.yaml` and
   `routes.json` open in the preview, next to the single binary `/whoami`.
3. **A stopped container.** Open `worker` (exited): the jobs it wrote in `/var/lib/worker/out`, and its logs up to
   `stop requested, exiting`.
4. **Secrets.** The Secrets page lists the names of what the agent masks, `db` included, never a value. Reveal
   `API_TOKEN`: one value at a time, and the reveal is written to the agent's audit log.
5. **Image and diff.** On the Images page, browse the files of `nginx:1.27.5-alpine` without starting a container.
   Then on `web`, the diff with the image marks `/etc/nginx/nginx.conf` as modified; its line diff shows
   `worker_connections` and `gzip`. The diff also lists `/etc/nginx/conf.d/default.conf` (rewritten by the entrypoint
   of the nginx image), `/run/nginx.pid` and `/var/cache/nginx/*`.
6. **Snapshot, change, compare.** Take a snapshot of `web`, edit `/usr/share/nginx/html/index.html` from the file
   explorer (the confirmation names the container and the path), then compare the snapshot with the current
   container: `index.html` is modified.
