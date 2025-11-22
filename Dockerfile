# Dockerfile for Lerian CLI
# Used by GoReleaser for multi-platform container images

FROM alpine:3.19

# Install runtime dependencies
RUN apk --no-cache add \
    ca-certificates \
    kubectl \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 lerian && \
    adduser -D -u 1000 -G lerian lerian

# Set working directory
WORKDIR /home/lerian

# Copy binary from GoReleaser
COPY lerian /usr/local/bin/lerian

# Change ownership
RUN chown -R lerian:lerian /home/lerian

# Switch to non-root user
USER lerian

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/lerian"]

# Default command
CMD ["--help"]

# Metadata
LABEL org.opencontainers.image.title="Lerian CLI" \
      org.opencontainers.image.description="Official command-line interface for the Lerian platform" \
      org.opencontainers.image.vendor="Lerian Studio" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.url="https://github.com/lerian-studio/lerian-cli" \
      org.opencontainers.image.source="https://github.com/lerian-studio/lerian-cli" \
      org.opencontainers.image.documentation="https://docs.lerian.studio"
