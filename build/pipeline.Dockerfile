FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH TARGETVARIANT
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} go build -o /out/pipeline ./cmd/pipeline

FROM alpine:3
RUN addgroup -g 1000 memory && adduser -D -u 1000 -G memory memory
COPY --from=build /out/pipeline /usr/local/bin/pipeline
USER memory:memory
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/pipeline"]
