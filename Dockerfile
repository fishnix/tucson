FROM golang:1.18-alpine AS builder
WORKDIR /app
RUN apk add --no-cache ca-certificates git
COPY go.* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=readonly -v -o out
RUN /app/out --help

FROM alpine:3 AS certs
RUN apk add --no-cache ca-certificates

FROM scratch
COPY --from=builder /app/out /app/tucson
COPY --from=alpine /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
WORKDIR /app
EXPOSE 8080

ENTRYPOINT ["/app/tucson"]
CMD ["serve"]