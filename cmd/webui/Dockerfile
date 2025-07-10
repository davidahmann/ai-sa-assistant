FROM golang:1.23.5-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
WORKDIR /app/cmd/webui
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/webui .

FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app

COPY --from=builder --chown=nonroot:nonroot /app/webui ./
COPY --from=builder --chown=nonroot:nonroot /app/configs/ ./configs/
COPY --from=builder --chown=nonroot:nonroot /app/cmd/webui/static/ ./static/
COPY --from=builder --chown=nonroot:nonroot /app/cmd/webui/templates/ ./templates/

USER nonroot

EXPOSE 8080
CMD ["./webui"]
