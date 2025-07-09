FROM golang:1.23.5-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
WORKDIR /app/cmd/teamsbot
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/teamsbot .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates && \
    addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

WORKDIR /app
USER appuser

COPY --from=builder --chown=appuser:appgroup /app/teamsbot ./
COPY --from=builder --chown=appuser:appgroup /app/configs/ ./configs/

EXPOSE 8080
CMD ["./teamsbot"]
