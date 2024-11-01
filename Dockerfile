# syntax=docker/dockerfile:1

## Builder
FROM golang:1.23.2-bookworm AS build
WORKDIR /app
ADD . .
ARG VERSION=dev
RUN go mod download
RUN CGO_ENABLED=0 go build -ldflags "-X main.Version=$VERSION" -o ll-bridge-api cli/main.go 

## Final Image
FROM alpine:3.20.3
WORKDIR /app/ll-bridge-api
COPY --from=build /app/ll-bridge-api .
EXPOSE 8080
CMD ["./ll-bridge-api"]
