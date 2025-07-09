FROM golang:1.23.5-alpine AS builder

RUN apk --no-cache add gcc musl-dev sqlite-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
WORKDIR /app/cmd/ingest
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o /app/ingest .

FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app

COPY --from=builder --chown=nonroot:nonroot /app/ingest ./
COPY --from=builder --chown=nonroot:nonroot /app/configs/ ./configs/

CMD ["./ingest"]
