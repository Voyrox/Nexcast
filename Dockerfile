FROM golang:1.26.1-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/netcast .

FROM alpine:3.21

WORKDIR /app

RUN addgroup -S netcast && adduser -S netcast -G netcast

COPY --from=builder /out/netcast /app/netcast

ENV SELF_ADDR=127.0.0.1:8081 \
    SERVICES_FILE=services.yaml \
    K8S_NAMESPACE=default \
    CHECK_INTERVAL=20s \
    COOLDOWN=60s \
    PUPPETS=

RUN mkdir -p /app/config/uploaded-sources && chown -R netcast:netcast /app

USER netcast

EXPOSE 8081

CMD ["/app/netcast"]
