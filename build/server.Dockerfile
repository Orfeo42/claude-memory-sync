FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM alpine:3
RUN apk add --no-cache git
RUN addgroup -g 1000 memory && adduser -D -u 1000 -G memory memory
RUN mkdir -p /data && chown -R memory:memory /data
COPY --from=build /out/server /usr/local/bin/server
USER memory:memory
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/server"]
