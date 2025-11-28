FROM golang:1.23.5-alpine AS builder

RUN apk add --no-cache build-base

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
WORKDIR /app/cmd/learning
RUN CGO_ENABLED=1 GOOS=linux go build -o /app/learning .

FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app

COPY --from=builder --chown=nonroot:nonroot /app/learning ./
COPY --from=builder --chown=nonroot:nonroot /app/configs/ ./configs/

USER nonroot

CMD ["./learning"]
