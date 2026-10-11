FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /my-assistant-gw .

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S -G app -u 10001 app \
    && mkdir -p /app \
    && chown app:app /app

WORKDIR /app
COPY --from=builder /my-assistant-gw /usr/local/bin/my-assistant-gw

USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/my-assistant-gw"]