FROM golang:1.19.5-buster AS builder

ARG major=0
ARG minor=0
ARG patch=0
ARG gitCommit

ENV GOPATH /go
ENV SRC_PATH $GOPATH/src/github.com/drand/drand-tools/
ENV GOPROXY https://proxy.golang.org

ENV SUEXEC_VERSION v0.2
ENV TINI_VERSION v0.19.0
RUN set -x \
  && cd /tmp \
  && git clone https://github.com/ncopa/su-exec.git \
  && cd su-exec \
  && git checkout -q $SUEXEC_VERSION \
  && make \
  && cd /tmp \
  && wget -q -O tini https://github.com/krallin/tini/releases/download/$TINI_VERSION/tini \
  && chmod +x tini

# Get the TLS CA certificates, they're not provided by busybox.
RUN apt-get update && apt-get install -y ca-certificates

COPY go.* $SRC_PATH
WORKDIR $SRC_PATH
RUN go mod download

COPY . $SRC_PATH
RUN \
  go build -o $GOPATH/bin/db-migration \
  -mod=readonly \
  -ldflags \
  "-X $(VER_PACKAGE).COMMIT=$(GIT_REVISION) \
    -X $(VER_PACKAGE).BUILDDATE=$(BUILD_DATE)" \
    ./cmd/db-migration

FROM busybox:1-glibc

ENV GOPATH     /go
ENV SRC_PATH   /go/src/github.com/drand/drand-tools
ENV DRAND_HOME /data/drand


COPY --from=builder $GOPATH/bin/db-migration /usr/local/bin/db-migration
COPY --from=builder $SRC_PATH/cmd/db-migration/docker/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY --from=builder /tmp/su-exec/su-exec /sbin/su-exec
COPY --from=builder /tmp/tini /sbin/tini
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

VOLUME $DRAND_HOME
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/entrypoint.sh"]

CMD [""]
