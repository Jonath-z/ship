ARG GO_BUILDER_IMAGE=golang:1.23.12-alpine3.22
ARG RUBY_IMAGE=ruby:3.4.9-alpine3.23

FROM ${GO_BUILDER_IMAGE} AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY server ./server
ARG BUILD_SHA=development
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/Jonath-z/ship/server/internal/platform/buildinfo.SHA=${BUILD_SHA} -X github.com/Jonath-z/ship/server/internal/platform/buildinfo.Version=${VERSION}" \
    -o /out/ship-worker ./server/cmd/worker

FROM ${RUBY_IMAGE}
ARG BUILD_SHA=development
ARG VERSION=dev
ARG KAMAL_VERSION=2.12.0
ARG SOURCE_URL=https://github.com/Jonath-z/ship
LABEL org.opencontainers.image.title="Ship Worker" \
      org.opencontainers.image.description="Ship worker with the pinned Kamal deployment runtime" \
      org.opencontainers.image.source="${SOURCE_URL}" \
      org.opencontainers.image.revision="${BUILD_SHA}" \
      org.opencontainers.image.version="${VERSION}" \
      dev.ship.kamal.version="${KAMAL_VERSION}"

RUN apk add --no-cache docker-cli docker-cli-buildx git openssh-client-default yaml \
    && apk add --no-cache --virtual .kamal-build build-base yaml-dev \
    && gem install kamal --version "${KAMAL_VERSION}" --no-document \
    && apk del .kamal-build \
    && rm -rf /root/.cache /usr/local/bundle/cache \
    && addgroup -S -g 10001 ship \
    && adduser -S -D -h /home/ship -u 10001 -G ship ship \
    && mkdir -p /data/ship /home/ship/.docker /home/ship/.ssh \
    && chown -R 10001:10001 /data/ship /home/ship

WORKDIR /data/ship
COPY --from=builder --chown=10001:10001 /out/ship-worker /ship-worker
VOLUME ["/data/ship"]
USER 10001:10001
EXPOSE 8081
HEALTHCHECK --interval=10s --timeout=6s --start-period=20s --retries=5 CMD ["/ship-worker", "-healthcheck"]
ENTRYPOINT ["/ship-worker"]
