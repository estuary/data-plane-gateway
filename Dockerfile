# Build Stage
################################################################################
FROM golang:1.18-buster as builder

WORKDIR /builder

RUN apt-get update && apt-get install -y openssl

# Download & compile dependencies early. Doing this separately allows for layer
# caching opportunities when no dependencies are updated.
COPY go.* ./
RUN go mod download

# Build the gateway.
COPY *.go ./
COPY gen ./gen
RUN go build .

# Generate a self-signed certificate to allow the server to use TLS
RUN openssl req -x509 -nodes -days 1095 \
    -subj "/C=CA/ST=QC/O=Estuary/CN=not-a-real-hostname.test" \
    -newkey rsa:2048 -keyout tls-private-key.pem \
    -out tls-cert.pem

# Runtime Stage
################################################################################
FROM gcr.io/distroless/base-debian10

WORKDIR /app
ENV PATH="/app:$PATH"

# Bring in the compiled artifact from the builder.
COPY --from=builder /builder/data-plane-gateway ./
COPY --from=builder /builder/tls-private-key.pem ./
COPY --from=builder /builder/tls-cert.pem ./

# Avoid running the data-plane-gateway as root.
USER nonroot:nonroot

ENTRYPOINT ["/app/data-plane-gateway"]
