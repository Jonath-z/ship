FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY server ./server
ARG BUILD_SHA=development
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/Jonath-z/ship/server/internal/platform/buildinfo.SHA=${BUILD_SHA} -X github.com/Jonath-z/ship/server/internal/platform/buildinfo.Version=${VERSION}" \
    -o /out/ship-worker ./server/cmd/worker

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/ship-worker /ship-worker
EXPOSE 8081
ENTRYPOINT ["/ship-worker"]
