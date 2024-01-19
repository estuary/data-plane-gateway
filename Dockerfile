# Build Stage
################################################################################
FROM golang as builder

WORKDIR /builder

RUN apt-get update && apt-get install -y openssl

# Download & compile dependencies early. Doing this separately allows for layer
# caching opportunities when no dependencies are updated.
COPY go.* ./
RUN go mod download

# Build the gateway.
COPY *.go ./
COPY gen ./gen
COPY auth ./auth
COPY proxy ./proxy
RUN go build .

# Generate a self-signed certificate to allow the server to use TLS
RUN openssl req -x509 -nodes -days 1095 \
    -subj "/C=CA/ST=QC/O=Estuary/CN=not-a-real-hostname.test" \
    -newkey rsa:2048 -keyout tls-private-key.pem \
    -out tls-cert.pem

# We'll copy the sh executable out of this, since distroless doesn't have a package manager with
# which to install one
FROM busybox:1.34-musl as busybox

# Runtime Stage
################################################################################
FROM gcr.io/distroless/base-debian11

COPY --from=busybox /bin/sh /bin/sh

WORKDIR /app
ENV PATH="/app:$PATH"

# Bring in the compiled artifact from the builder.
COPY --from=builder /builder/data-plane-gateway ./
COPY --from=builder --chown=nonroot /builder/tls-private-key.pem ./
COPY --from=builder --chown=nonroot /builder/tls-cert.pem ./

# Avoid running the data-plane-gateway as root.
USER nonroot:nonroot

ENTRYPOINT ["/app/data-plane-gateway"]
