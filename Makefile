.PHONY: build run docker-build docker-up docker-down dev

# Build binary
build:
	CGO_ENABLED=1 go build -o bin/ngate ./main.go

# Run locally (requires nginx installed)
run: build
	sudo ./bin/ngate -port 8080

# Dev mode - run with local paths
dev: build
	mkdir -p /tmp/npm-data /tmp/npm-conf /tmp/npm-certs
	sudo ./bin/ngate \
		-port 8080 \
		-data /tmp/npm-data \
		-conf /tmp/npm-conf \
		-certs /tmp/npm-certs

# Docker
docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f

# Test nginx config
test-nginx:
	sudo nginx -t
