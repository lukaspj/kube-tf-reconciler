FROM golang:1.24-alpine AS build

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

# Create non-root user
RUN addgroup -g 1000 krec && \
    adduser -u 1000 -G krec -s /bin/sh -D krec

# Create workspace directory structure with proper permissions
RUN mkdir -p /tmp/workspaces/installs && \
    chown -R krec:krec /tmp/workspaces && \
    chmod -R 775 /tmp/workspaces

COPY --from=build /src/krec /usr/local/bin/krec
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Switch to non-root user
USER 1000
ENTRYPOINT ["krec", "operator"]