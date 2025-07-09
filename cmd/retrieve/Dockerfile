FROM golang:1.23.5-alpine AS builder

RUN apk --no-cache add gcc musl-dev sqlite-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
WORKDIR /app/cmd/retrieve
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o /app/retrieve .

FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app

COPY --from=builder --chown=nonroot:nonroot /app/retrieve ./
COPY --from=builder --chown=nonroot:nonroot /app/configs/ ./configs/

EXPOSE 8080
CMD ["./retrieve"]
