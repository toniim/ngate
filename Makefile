.PHONY: build run docker-build docker-up docker-down dev dev-down deploy

HOST ?= tonysproxy

# Build binary
build:
	CGO_ENABLED=1 go build -o bin/ngate ./main.go

# Run locally (requires nginx installed)
run: build
	sudo ./bin/ngate -port 8080

# Dev mode - docker with hot reload via air
dev:
	docker compose -f docker-compose.dev.yml up --build

dev-down:
	docker compose -f docker-compose.dev.yml down

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

# Deploy to remote host: make deploy HOST=tonysproxy
deploy:
	rsync -avz --delete \
		--exclude='data/' --exclude='.git/' --exclude='.claude/' --exclude='.serena/' --exclude='plans/' --exclude='bin/' \
		./ $(HOST):~/ngate/
	ssh $(HOST) "cd ~/ngate && docker compose up -d --build"
