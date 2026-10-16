# Release image, built by GoReleaser (dockers_v2, .goreleaser.yaml): the build
# context holds linux/amd64/hublot and linux/arm64/hublot, already compiled
# (static, CGO_ENABLED=0). It does not build on its own: see scripts/check-image.sh
# and the release-snapshot CI job.
#
# FROM scratch: nothing but the binary. The agent runs as root on purpose
# (spec §7, §8): it reads /proc/<pid>/root of the containers.
FROM scratch
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/hublot /hublot
ENV HUBLOT_LISTEN=0.0.0.0:7878 HUBLOT_DATA_DIR=/data
EXPOSE 7878
ENTRYPOINT ["/hublot"]
