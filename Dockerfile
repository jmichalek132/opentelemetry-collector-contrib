FROM golang:1.19.1-alpine3.16 AS build-env

ENV CGO_ENABLED 0

# Allow Go to retreive the dependencies for the build step
RUN apk add --no-cache git

WORKDIR /otel/
ADD . /otel/

RUN cd ./cmd/otelcontribcol && GO111MODULE=on CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -o /srv .

# Get Delve from a GOPATH not from a Go Modules project
WORKDIR /go/

# final stage
FROM alpine:3.16

WORKDIR /
COPY --from=build-env /srv/otelcontribcol /
EXPOSE 8080 40000

CMD ["/otelcontribcol", "--", "--config", "/conf/relay.yaml"]

