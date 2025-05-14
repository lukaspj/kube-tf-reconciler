FROM golang:1.24-alpine AS build

RUN apk update && apk upgrade && apk add --no-cache ca-certificates git
RUN update-ca-certificates

# Install Delve debugger from source
RUN go install github.com/go-delve/delve/cmd/dlv@latest

ARG SHA
ARG VERSION
ARG DATE

COPY . /src
WORKDIR /src

# Build with debugging information
RUN CGO_ENABLED=0 go build -gcflags="all=-N -l" -ldflags "-X cmd.commit=$SHA -X cmd.version=$VERSION -X cmd.date=$DATE" -o krec main.go

FROM alpine:3.19

# Install required tools and debugging tools
RUN apk add --no-cache ca-certificates git

# Create non-root user
RUN addgroup -g 1000 krec && \
    adduser -u 1000 -G krec -s /bin/sh -D krec

# Create workspace directory structure with proper permissions
RUN mkdir -p /tmp/workspaces/installs && \
    chown -R krec:krec /tmp/workspaces && \
    chmod -R 775 /tmp/workspaces

COPY --from=build /src/krec /usr/local/bin/krec
COPY --from=build /go/bin/dlv /usr/local/bin/dlv
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Expose delve API port
EXPOSE 2345

# Switch to non-root user
USER 1000

# Start delve
ENTRYPOINT ["dlv", "--listen=:2345", "--headless=true", "--api-version=2", "--accept-multiclient", "exec", "/usr/local/bin/krec", "--", "operator"]