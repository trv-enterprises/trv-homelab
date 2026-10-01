FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /sentinel .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata wget
COPY --from=build /sentinel /usr/local/bin/sentinel
VOLUME /data
EXPOSE 8070
ENTRYPOINT ["sentinel"]
