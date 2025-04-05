# Go Router

An HTTP and WebSocket reverse proxy router written in Go.

## Features

- HTTP request routing and forwarding
- WebSocket connection forwarding
- YAML configuration support
- Multiple route configuration
- Support for listening on multiple ports simultaneously
- Graceful shutdown support
- Colored terminal log output
- Docker support with host network mode

## Configuration

Create a `config.yaml` file in the project root directory with the following structure:

```yaml
router:
  - server: 8080 # First server listening port
    redirect:
      - path: "/server_a"
        port: 1234
      - path: "/server_b"
        port: 5678
      - path: "/server_c"
        port: 9012
  - server: 8081 # Second server listening port
    redirect:
      - path: "/server_a"
        port: 1235
      - path: "/server_b"
        port: 5679
      - path: "/server_c"
        port: 9013
```

- `router`: List of router server configurations
  - `server`: Port to listen on
  - `redirect`: List of forwarding rules
    - `path`: URL path prefix to match
    - `port`: Target port to forward to

## Usage

### Running Locally

1. Install dependencies:

```bash
go mod download
```

2. Run the server:

```bash
go run main.go
```

The server will start and listen on all configured ports. All HTTP and WebSocket requests matching the configured paths will be forwarded to their respective target ports.

### Running with Docker

This project includes Docker support with host network mode to ensure proper port forwarding functionality.

1. Make sure you have Docker and docker-compose installed on your system
2. Use the provided Makefile commands for easy container management:

```bash
# Start in background
make up

# Start in foreground (view logs)
make up-fg

# Stop the container
make down

# Rebuild and restart
make reset

# View logs
make logs

# Clean up resources
make clean
```

For a complete list of available commands:

```bash
make help
```

## Makefile Commands

The project includes a Makefile to simplify Docker operations:

| Command      | Description                                      |
| ------------ | ------------------------------------------------ |
| `make up`    | Start containers (background)                    |
| `make up-fg` | Start containers (foreground, view logs)         |
| `make down`  | Stop containers                                  |
| `make reset` | Rebuild and start containers                     |
| `make build` | Build images only                                |
| `make logs`  | View container logs                              |
| `make clean` | Clean environment (remove containers and images) |
| `make help`  | Display help information                         |

## Graceful Shutdown

The program supports graceful shutdown. When it receives a SIGINT (Ctrl+C) or SIGTERM signal, the server will:

1. Stop accepting new connections
2. Wait for existing requests to complete processing (maximum 10 seconds)
3. Safely shut down all servers

## Example

If you have the following configuration:

```yaml
router:
  - server: 8080
    redirect:
      - path: "/api"
        port: 9000
      - path: "/ws"
        port: 9001
  - server: 8081
    redirect:
      - path: "/api"
        port: 9002
```

- HTTP requests to `http://localhost:8080/api/*` will be forwarded to `http://localhost:9000/*`
- WebSocket connections to `ws://localhost:8080/ws/*` will be forwarded to `ws://localhost:9001/*`
- HTTP requests to `http://localhost:8081/api/*` will be forwarded to `http://localhost:9002/*`

## Dependencies

- [gorilla/websocket](https://github.com/gorilla/websocket) - WebSocket support
- [spf13/viper](https://github.com/spf13/viper) - Configuration file handling
