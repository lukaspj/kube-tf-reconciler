FROM golang:1.22-alpine AS build

RUN apk update && apk upgrade && apk add --no-cache ca-certificates git
RUN update-ca-certificates

ARG SHA
ARG VERSION
ARG DATE

COPY . /src
WORKDIR /src

RUN CGO_ENABLED=0 go build -ldflags "-X cmd.commit=$SHA -X cmd.version=$VERSION -X cmd.date=$DATE" -o krec main.go

FROM alpine:3.19 AS krec
# Install required tools
RUN apk add --no-cache ca-certificates git

# Create workspace directory structure
RUN mkdir -p /tmp/workspaces/installs && \
    chmod -R 777 /tmp/workspaces

COPY --from=build /src/krec /usr/local/bin/krec
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["krec", "operator"]