FROM golang:1.26-alpine

# Kamal must match the production worker image (infra/versions.env) so dev
# deployments exercise the same engine the release ships.
ARG KAMAL_VERSION=2.12.0

RUN apk add --no-cache git ca-certificates ruby docker-cli docker-cli-buildx openssh-client-default yaml \
    && apk add --no-cache --virtual .kamal-build build-base ruby-dev yaml-dev \
    && gem install kamal --version "${KAMAL_VERSION}" --no-document \
    && apk del .kamal-build \
    && rm -rf /root/.cache \
    && go install github.com/air-verse/air@v1.61.7

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download

COPY . .
