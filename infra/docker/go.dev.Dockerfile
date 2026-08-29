FROM golang:1.23-alpine

RUN apk add --no-cache git ca-certificates \
    && go install github.com/air-verse/air@v1.61.7

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download

COPY . .
