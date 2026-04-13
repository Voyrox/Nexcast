FROM golang:1.26-alpine AS build
WORKDIR /app
COPY . .
RUN go build -o /out/example-server main.go

FROM alpine:3.21
WORKDIR /app
COPY --from=build /out/example-server /app/example-server
EXPOSE 8080
CMD ["/app/example-server"]
