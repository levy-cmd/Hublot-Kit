# Image des tests Linux (scripts/test-linux.sh) : Go + client Docker.
FROM golang:1.27
COPY --from=docker:cli /usr/local/bin/docker /usr/local/bin/docker
