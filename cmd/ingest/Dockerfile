FROM golang:1.23.5-alpine AS builder

RUN apk --no-cache add gcc musl-dev sqlite-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
WORKDIR /app/cmd/ingest
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o /app/ingest .

FROM alpine:3.18

RUN apk --no-cache add ca-certificates sqlite && \
    adduser -D -s /bin/sh appuser

WORKDIR /app

COPY --from=builder --chown=appuser:appuser /app/ingest ./ingest
COPY --from=builder --chown=appuser:appuser /app/configs/ ./configs/

# Create data directory with correct ownership for the volume mount
RUN mkdir -p /app/data && chown appuser:appuser /app/data

USER appuser

CMD ["./ingest"]
