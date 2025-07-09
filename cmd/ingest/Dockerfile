FROM golang:1.23.5-alpine AS builder

RUN apk --no-cache add gcc musl-dev sqlite-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
WORKDIR /app/cmd/ingest
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o /app/ingest .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates sqlite && \
    addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

WORKDIR /app
USER appuser

COPY --from=builder --chown=appuser:appgroup /app/ingest ./
COPY --from=builder --chown=appuser:appgroup /app/configs/ ./configs/

CMD ["./ingest"]
