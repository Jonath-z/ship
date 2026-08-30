ARG GO_BUILDER_IMAGE=golang:1.23.12-alpine3.22
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian12:nonroot

FROM ${GO_BUILDER_IMAGE} AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY server ./server
ARG BUILD_SHA=development
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/Jonath-z/ship/server/internal/platform/buildinfo.SHA=${BUILD_SHA} -X github.com/Jonath-z/ship/server/internal/platform/buildinfo.Version=${VERSION}" \
    -o /out/ship-api ./server/cmd/api

FROM ${RUNTIME_IMAGE}
ARG BUILD_SHA=development
ARG VERSION=dev
ARG SOURCE_URL=https://github.com/Jonath-z/ship
LABEL org.opencontainers.image.title="Ship API" \
      org.opencontainers.image.description="Ship control-plane API" \
      org.opencontainers.image.source="${SOURCE_URL}" \
      org.opencontainers.image.revision="${BUILD_SHA}" \
      org.opencontainers.image.version="${VERSION}"
COPY --from=builder /out/ship-api /ship-api
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=6s --start-period=20s --retries=5 CMD ["/ship-api", "-healthcheck"]
ENTRYPOINT ["/ship-api"]
