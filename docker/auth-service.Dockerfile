FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY shared /app/shared
COPY services/auth-service /app/services/auth-service
WORKDIR /app/services/auth-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -o /out/server ./cmd/server

FROM alpine:3.22
WORKDIR /app
COPY --from=builder /out/server /app/server
EXPOSE 8001
CMD ["/app/server"]
