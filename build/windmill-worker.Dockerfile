FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/memoryctl ./cmd/memoryctl

FROM ghcr.io/windmill-labs/windmill:main
USER root
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates curl && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && apt-get install -y --no-install-recommends nodejs && rm -rf /var/lib/apt/lists/* && npm install -g @anthropic-ai/claude-code
COPY --from=build /out/memoryctl /usr/local/bin/memoryctl
