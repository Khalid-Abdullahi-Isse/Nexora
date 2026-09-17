FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY shared /app/shared
COPY services/notification-service /app/services/notification-service
WORKDIR /app/services/notification-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -buildvcs=false -o /out/server ./cmd/server

FROM alpine:3.22
RUN addgroup -g 10001 app && adduser -D -u 10001 -G app app
WORKDIR /app
COPY --from=builder /out/server /app/server
EXPOSE 8005
USER app
CMD ["/app/server"]
