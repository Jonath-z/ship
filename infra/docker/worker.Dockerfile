ARG GO_BUILDER_IMAGE=golang:1.26.8-alpine3.23
ARG RUBY_IMAGE=ruby:3.4.9-alpine3.23
ARG ALPINE_IMAGE=alpine:3.23.5

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

# Prepare the Kamal runtime here, then copy only the slimmed /usr/local into a
# fresh base below. The ruby image ships libruby and every native extension
# with debug symbols plus bundled development gems Kamal never loads; stripping
# or deleting them in place would not shrink the base layers, so the final
# stage must start clean for the image to stay under the 200 MiB contract limit.
FROM ${RUBY_IMAGE} AS kamal
ARG KAMAL_VERSION=2.12.0
RUN apk add --no-cache --virtual .kamal-build build-base yaml-dev \
    && gem install kamal --version "${KAMAL_VERSION}" --no-document \
    && strip /usr/local/lib/libruby.so.* \
    && find /usr/local/lib/ruby /usr/local/bundle -name '*.so' -exec strip {} + \
    && for gem in rbs nkf debug net-imap rexml test-unit power_assert typeprof syslog rss csv repl_type_completor; do \
         rm -rf /usr/local/lib/ruby/gems/*/gems/"${gem}"-* \
                /usr/local/lib/ruby/gems/*/specifications/"${gem}"-*.gemspec \
                /usr/local/lib/ruby/gems/*/extensions/*/*/"${gem}"-*; \
       done \
    && rm -f /usr/local/bin/rbs /usr/local/bin/rdbg /usr/local/bin/typeprof \
    && rm -rf /usr/local/lib/ruby/gems/*/cache /usr/local/bundle/cache /root/.cache

FROM ${ALPINE_IMAGE}
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

ENV GEM_HOME=/usr/local/bundle \
    BUNDLE_SILENCE_ROOT_WARNING=1 \
    BUNDLE_APP_CONFIG=/usr/local/bundle \
    PATH=/usr/local/bundle/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# gmp, libffi, libgcc, yaml, and zlib are ruby's shared-library dependencies;
# the openssl libraries already ship with the Alpine base.
RUN apk add --no-cache docker-cli docker-cli-buildx git openssh-client-default yaml gmp libffi libgcc zlib \
    && addgroup -S -g 10001 ship \
    && adduser -S -D -h /home/ship -u 10001 -G ship ship \
    && mkdir -p /data/ship /home/ship/.docker /home/ship/.ssh \
    && chown -R 10001:10001 /data/ship /home/ship

COPY --from=kamal /usr/local/bin /usr/local/bin
COPY --from=kamal /usr/local/lib /usr/local/lib
COPY --from=kamal /usr/local/bundle /usr/local/bundle

WORKDIR /data/ship
COPY --from=builder --chown=10001:10001 /out/ship-worker /ship-worker
VOLUME ["/data/ship"]
USER 10001:10001
EXPOSE 8081
HEALTHCHECK --interval=10s --timeout=6s --start-period=20s --retries=5 CMD ["/ship-worker", "-healthcheck"]
ENTRYPOINT ["/ship-worker"]
