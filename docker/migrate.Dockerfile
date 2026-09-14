FROM golang:1.27-alpine AS builder
WORKDIR /app/shared
COPY shared/ ./
RUN CGO_ENABLED=0 GOWORK=off go build -buildvcs=false -o /out/migrate ./cmd/migrate

FROM alpine:3.22
RUN addgroup -g 10001 app && adduser -D -u 10001 -G app app
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/migrate /app/migrate
COPY docker/migrations/legacy-user /app/docker/migrations/legacy-user
COPY services/auth-service/migrations /app/services/auth-service/migrations
COPY services/post-service/migrations /app/services/post-service/migrations
COPY services/chat-service/migrations /app/services/chat-service/migrations
COPY services/notification-service/migrations /app/services/notification-service/migrations
ENTRYPOINT ["/app/migrate"]
USER app
CMD ["all", "up"]
