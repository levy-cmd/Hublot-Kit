#!/usr/bin/env bash
# Hublot-Kit demo (see README.md in this folder).
#   ./demo.sh up    start the demo containers and the agent, then set the scene
#   ./demo.sh down  remove what the compose file created (containers, network, volume)
# bash 3.2 or later.
set -euo pipefail
cd "$(dirname "$0")"
url=http://127.0.0.1:7878
project=hublot-demo
# An exported COMPOSE_PROJECT_NAME would win over the name: of compose.yaml.
unset COMPOSE_PROJECT_NAME

compose() { docker compose -p "$project" -f compose.yaml "$@"; }

# Stop, touching nothing, if a container takes a name of the demo without
# belonging to it. A container of an earlier run of the demo is accepted.
check_names() {
  local name found owner
  for name in api worker web db hublot-demo; do
    # One line "found:<project>" per container with this exact name, none if free.
    found=$(docker ps -a --filter "name=^/${name}\$" \
      --format 'found:{{.Label "com.docker.compose.project"}}')
    if [ -n "$found" ] && [ "$found" != "found:$project" ]; then
      owner=${found#found:}
      echo "A container named $name exists and is not part of the demo (Compose project: ${owner:-none})." >&2
      echo "Rename or remove it, then run ./demo.sh up again. Nothing was changed." >&2
      exit 1
    fi
  done
}

up() {
  check_names
  since=$(date +%s)
  compose up -d
  # api has no shell: docker cp goes through the Docker API, not the container.
  docker cp api/. api:/etc/api
  # worker writes a few jobs and logs, then is stopped.
  for _ in $(seq 1 30); do
    # grep without -q reads to the end: no SIGPIPE for docker logs under pipefail.
    docker logs --since "$since" worker 2>&1 | grep 'job 6 done' >/dev/null && break
    sleep 1
  done
  compose stop worker
  # web: nginx.conf now differs from the one of the image.
  docker exec web sed -i -e 's/worker_connections  1024;/worker_connections  4096;/' \
    -e 's/#gzip  on;/gzip  on;/' /etc/nginx/nginx.conf
  docker exec web nginx -s reload
  code=
  for _ in $(seq 1 30); do
    code=$(curl -s -o /dev/null -w '%{http_code}' "$url/login" || true)
    [ "$code" = 200 ] && break
    sleep 1
  done
  if [ "$code" != 200 ]; then
    echo "The agent does not answer on $url (HTTP $code):" >&2
    docker logs hublot-demo 2>&1 | tail -n 20 >&2
    exit 1
  fi
  cat <<EOF

Hublot-Kit is ready on $url
The access token is printed once, on the first start:

  docker logs hublot-demo

Token lost? docker exec hublot-demo /hublot reset-token && docker restart hublot-demo
Try: docker exec -it api sh   (fails: the image has no shell)
Clean up: ./demo.sh down
EOF
}

case ${1:-} in
  up) up ;;
  down) compose down -v ;;
  *) echo "usage: $0 up | down" >&2; exit 2 ;;
esac
