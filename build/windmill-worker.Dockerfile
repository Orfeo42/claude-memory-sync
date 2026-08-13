FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/memoryctl ./cmd/memoryctl

FROM ghcr.io/windmill-labs/windmill:main
USER root
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && rm -rf /var/lib/apt/lists/*
RUN rm -f /usr/bin/claude /usr/local/bin/claude && npm install -g @anthropic-ai/claude-code && claude --version
RUN git config --system --add safe.directory '*'
COPY --from=build /out/memoryctl /usr/local/bin/memoryctl
