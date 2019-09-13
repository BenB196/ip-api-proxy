FROM golang:latest as builder
WORKDIR /app
COPY go.mod go.sum ./
COPY . .
RUN go clean
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o ip-api-proxy .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
RUN mkdir -p etc/ip-api-proxy
RUN mkdir -p /ip-api-proxy && chown -R nobody:nogroup etc/crashplan-ffs-puller /crashplan-ffs-puller

USER nobody

VOLUME ["/ip-api-proxy"]
WORKDIR /ip-api-proxy
COPY --from=builder /app/ip-api-proxy /bin/ip-api-proxy

ENTRYPOINT ["/usr/sbin/update-ca-certificates & /bin/crashplan-ffs-puller"]
CMD ["--config=/etc/ip-api-proxy/config.json"]
