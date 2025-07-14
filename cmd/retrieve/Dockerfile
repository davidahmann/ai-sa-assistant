FROM golang:1.23.5-alpine AS builder

RUN apk --no-cache add gcc musl-dev sqlite-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
WORKDIR /app/cmd/retrieve
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o /app/retrieve .

# Verify the binary was created
RUN ls -la /app/retrieve

FROM alpine:3.18

RUN apk --no-cache add ca-certificates=20240705-r0 sqlite=3.46.1-r0 && \
    adduser -D -s /bin/sh appuser

WORKDIR /app

COPY --from=builder --chown=appuser:appuser /app/retrieve ./retrieve
COPY --from=builder --chown=appuser:appuser /app/configs/ ./configs/

# Create data directory with correct ownership for the volume mount
RUN mkdir -p /app/data && chown appuser:appuser /app/data

USER appuser

EXPOSE 8081
CMD ["./retrieve"]
