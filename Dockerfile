FROM golang:1.25-bookworm AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /ngate ./main.go

# ---
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    nginx \
    ca-certificates \
    curl \
    libnss3-tools \
    && rm -rf /var/lib/apt/lists/*

# Install mkcert
RUN curl -JLO "https://dl.filippo.io/mkcert/latest?for=linux/amd64" \
    && chmod +x mkcert-v*-linux-amd64 \
    && mv mkcert-v*-linux-amd64 /usr/local/bin/mkcert

# Setup nginx
RUN rm -f /etc/nginx/sites-enabled/default
COPY nginx/nginx.conf /etc/nginx/nginx.conf
COPY nginx/default /usr/share/ngate/default

COPY --from=builder /ngate /usr/local/bin/ngate

# Create dirs
RUN mkdir -p /etc/ngate/certs /etc/nginx/sites-enabled /var/www/acme

# Init mkcert CA
RUN mkcert -install

EXPOSE 80 443 8080

COPY scripts/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
