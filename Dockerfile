FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/worker ./cmd/worker

FROM alpine:3.22
RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D app
WORKDIR /app
COPY --from=builder --chown=app:app /out/server /app/server
COPY --from=builder --chown=app:app /out/worker /app/worker
COPY --from=builder --chown=app:app /app/web /app/web
USER app
CMD ["/app/server"]
