#!/bin/bash
set -e

echo "==> Starting Nginx..."
nginx

echo "==> Starting Proxy Manager on :8080..."
exec ngate \
  -port 8080 \
  -data /etc/ngate \
  -conf /etc/nginx/sites-enabled \
  -certs /etc/ngate/certs
