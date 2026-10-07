ARG BASE=gcr.io/distroless/static-debian12

FROM golang:1 AS sleeper
RUN printf 'package main\nimport "time"\nfunc main() { for { time.Sleep(time.Hour) } }\n' > /m.go \
 && CGO_ENABLED=0 go build -o /sleep /m.go

FROM busybox:1.36 AS tree
RUN mkdir -p /out/usr/many /out/odd /out/tmp \
 && i=1; while [ $i -le 1000 ]; do touch /out/usr/many/f$i; i=$((i+1)); done \
 && touch "/out/odd/with space" "/out/odd/émoji-🎉" \
 && ln -s /tmp/HOST_MARKER /out/evil \
 && ln -s ../../../../../tmp/HOST_MARKER /out/evil-rel

FROM ${BASE}
COPY --from=tree /out/ /
COPY --from=sleeper /sleep /sleep
ENTRYPOINT ["/sleep"]
