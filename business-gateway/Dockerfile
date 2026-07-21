FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/business-gateway ./cmd/business-gateway

FROM alpine:3.22

RUN addgroup -S gateway && adduser -S -G gateway gateway
WORKDIR /app
COPY --from=builder /out/business-gateway /app/business-gateway
RUN mkdir -p /data && chown gateway:gateway /data

USER gateway
EXPOSE 8080
ENTRYPOINT ["/app/business-gateway"]
