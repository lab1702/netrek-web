# Use latest Go version
FROM golang

# Install the NETREK-WEB server
RUN go install github.com/lab1702/netrek-web@latest

# Game available on port 8080
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run server
CMD ["netrek-web"]
