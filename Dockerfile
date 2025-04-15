FROM golang:1.24-alpine AS build

RUN apk update && apk upgrade && apk add --no-cache ca-certificates
RUN update-ca-certificates

ARG SHA
ARG VERSION

COPY . /src
WORKDIR /src

RUN CGO_ENABLED=0 go build -ldflags "-X main.sha=$SHA -X main.version=$VERSION" -o krec main.go

FROM scratch AS krec
COPY --from=build /src/krec /usr/local/bin/krec
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["krec", "operator"]
