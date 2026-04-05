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

COPY bin/ngate /usr/local/bin/ngate

# Create dirs
RUN mkdir -p /etc/ngate/certs /etc/nginx/sites-enabled /var/www/acme

# Redirect nginx logs to Docker stdout/stderr
RUN ln -sf /dev/stdout /var/log/nginx/access.log \
    && ln -sf /dev/stderr /var/log/nginx/error.log

# Init mkcert CA
RUN mkcert -install

EXPOSE 80 443 8080

COPY scripts/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
