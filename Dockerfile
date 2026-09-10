# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o status-page .

# ---
FROM alpine:latest
ENV TZ=Asia/Jakarta

WORKDIR /app
COPY --from=builder --chmod=755 /app/status-page .
COPY --from=builder /app/incidents.json ./incidents.json

EXPOSE 8086

HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8086/health >/dev/null 2>&1 || exit 1

CMD ["./status-page"]