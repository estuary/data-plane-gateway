# Build Stage
################################################################################
FROM golang:1.18-buster as builder

WORKDIR /builder

# Download & compile dependencies early. Doing this separately allows for layer
# caching opportunities when no dependencies are updated.
COPY go.* ./
RUN go mod download

# Build the gateway.
COPY *.go ./
COPY gen ./gen
RUN go build .


# Runtime Stage
################################################################################
FROM gcr.io/distroless/base-debian10

WORKDIR /app
ENV PATH="/app:$PATH"

# Bring in the compiled artifact from the builder.
COPY --from=builder /builder/data-plane-gateway ./data-plane-gateway

# Avoid running the data-plane-gateway as root.
USER nonroot:nonroot

ENTRYPOINT ["/app/data-plane-gateway"]
