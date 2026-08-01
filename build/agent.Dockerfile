FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH TARGETVARIANT
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} go build -o /out/agent ./cmd/agent

FROM alpine:3
RUN apk add --no-cache git
RUN addgroup -g 1000 memory && adduser -D -u 1000 -G memory memory
RUN mkdir -p /claude /state && chown -R memory:memory /claude /state
COPY --from=build /out/agent /usr/local/bin/agent
USER memory:memory
ENTRYPOINT ["/usr/local/bin/agent"]
