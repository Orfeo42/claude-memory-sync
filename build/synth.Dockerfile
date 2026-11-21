FROM --platform=$BUILDPLATFORM node:26.13-trixie-slim AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-trixie AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/synth ./cmd/synth

FROM node:26.13-trixie-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
RUN npm install -g @anthropic-ai/claude-code && claude --version
RUN git config --system --add safe.directory '*' \
    && git config --system maintenance.autoDetach false \
    && git config --system gc.autoDetach false
ENV HOME=/home/node
ENV SYNTH_WEB_DIR=/srv/web
COPY --from=web /web/dist /srv/web
COPY --from=build /out/synth /usr/local/bin/synth
USER 1000:1000
EXPOSE 8082
ENTRYPOINT ["/usr/local/bin/synth"]
