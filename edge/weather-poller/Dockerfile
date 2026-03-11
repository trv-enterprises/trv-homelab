FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
RUN CGO_ENABLED=0 go build -o /weather-poller .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /weather-poller /usr/local/bin/weather-poller
ENTRYPOINT ["weather-poller"]
