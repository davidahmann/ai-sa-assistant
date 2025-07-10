FROM golang:1.23.5-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
WORKDIR /app/cmd/teamsbot
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/teamsbot .

FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app

COPY --from=builder --chown=nonroot:nonroot /app/teamsbot ./
COPY --from=builder --chown=nonroot:nonroot /app/configs/ ./configs/

USER nonroot

EXPOSE 8080
CMD ["./teamsbot"]
