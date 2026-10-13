ARG GO_BUILDER_IMAGE=golang:1.26.8-alpine3.23
ARG NODE_IMAGE=node:22.23.2-alpine3.23
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

# The dashboard is a static Next.js export served by ship-api itself; no Node
# runtime ships in the final image.
FROM ${NODE_IMAGE} AS webbuilder
RUN corepack enable && corepack prepare pnpm@9.12.0 --activate
WORKDIR /workspace
COPY . .
RUN pnpm install --frozen-lockfile && pnpm --filter @ship/web build

FROM ${RUNTIME_IMAGE}
ARG BUILD_SHA=development
ARG VERSION=dev
ARG SOURCE_URL=https://github.com/Jonath-z/ship
LABEL org.opencontainers.image.title="Ship API" \
      org.opencontainers.image.description="Ship control-plane API and dashboard" \
      org.opencontainers.image.source="${SOURCE_URL}" \
      org.opencontainers.image.revision="${BUILD_SHA}" \
      org.opencontainers.image.version="${VERSION}"
COPY --from=builder /out/ship-api /ship-api
COPY --from=webbuilder /workspace/apps/web/out /web
ENV SHIP_WEB_DIR=/web
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=6s --start-period=20s --retries=5 CMD ["/ship-api", "-healthcheck"]
ENTRYPOINT ["/ship-api"]
