FROM golang:latest as builder
WORKDIR /app
COPY go.mod go.sum ./
COPY . .
RUN go clean
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o ip-api-proxy .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/ip-api-proxy .

EXPOSE 8080
CMD ["./ip-api-proxy"]