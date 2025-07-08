FROM golang:1.23.5-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
WORKDIR /app/cmd/synthesize
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/synthesize .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/synthesize .
COPY --from=builder /app/configs/ ./configs/

EXPOSE 8080
CMD ["./synthesize"]
