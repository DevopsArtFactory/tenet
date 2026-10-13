FROM golang:1.27.2-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/tenet-api ./cmd/tenet-api
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/tenet-controller ./cmd/tenet-controller

FROM alpine:3.20 AS runtime
RUN adduser -D -H -u 10001 tenet
WORKDIR /app
COPY --from=builder /out/tenet-api /usr/local/bin/tenet-api
COPY --from=builder /out/tenet-controller /usr/local/bin/tenet-controller
USER tenet
CMD ["/usr/local/bin/tenet-api"]
