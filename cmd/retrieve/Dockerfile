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

RUN apk --no-cache add ca-certificates sqlite
RUN adduser -D -s /bin/sh appuser

WORKDIR /app

COPY --from=builder --chown=appuser:appuser /app/retrieve ./retrieve
COPY --from=builder --chown=appuser:appuser /app/configs/ ./configs/

USER appuser

EXPOSE 8081
CMD ["./retrieve"]
