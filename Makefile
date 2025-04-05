# Go Router Makefile
# Convenient commands for Docker operations

.PHONY: up down reset build logs clean help

# Default target
.DEFAULT_GOAL := help

# Display help information
help:
	@echo "Go Router Makefile Help"
	@echo "------------------------"
	@echo "make up        - Start containers (background)"
	@echo "make up-fg     - Start containers (foreground, view logs)"
	@echo "make down      - Stop containers"
	@echo "make reset     - Rebuild and start containers"
	@echo "make build     - Build images only"
	@echo "make logs      - View container logs"
	@echo "make clean     - Clean environment (remove containers and images)"
	@echo "make help      - Display this help information" 

# Start containers (background)
up:
	docker-compose up -d

# Start containers (foreground)
up-fg:
	docker-compose up

# Stop containers
down:
	docker-compose down

# Rebuild and start containers
reset: down
	docker-compose up -d --build

# Build images only
build:
	docker-compose build

# View logs
logs:
	docker-compose logs -f

# Clean environment (remove containers, images and related resources)
clean: down
	docker-compose rm -f
	docker rmi $$(docker images -q go-router:latest) 2>/dev/null || true