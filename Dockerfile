FROM ubuntu:24.04

WORKDIR /app
ENV PATH="/app:$PATH"

# Install ca-certificates
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Bring in the compiled artifact from the builder.
COPY data-plane-gateway ./

# Avoid running the data-plane-gateway as root.
USER 65534:65534

# Ensure data-plane-gateway can run on this runtime image.
RUN /app/data-plane-gateway print-config

ENTRYPOINT ["/app/data-plane-gateway"]
