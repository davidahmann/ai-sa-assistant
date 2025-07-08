FROM golang:1.23.5-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
WORKDIR /app/cmd/ingest
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o /app/ingest .

FROM alpine:latest
RUN apk --no-cache add ca-certificates sqlite
WORKDIR /root/

COPY --from=builder /app/ingest .
COPY --from=builder /app/configs/ ./configs/

CMD ["./ingest"]
