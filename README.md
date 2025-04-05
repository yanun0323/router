# Go Router

A simple HTTP and WebSocket reverse proxy router written in Go.

## Features

- HTTP request routing
- WebSocket connection forwarding
- YAML configuration support
- Multiple route support

## Configuration

Create a `config.yaml` file in the project root with the following structure:

```yaml
routes:
  - path: "/api"
    port: 8081
  - path: "/ws"
    port: 8082
server:
  port: 8080
```

- `routes`: List of route configurations
  - `path`: URL path prefix to match
  - `port`: Target port to forward requests to
- `server`: Server configuration
  - `port`: Port to listen on

## Usage

1. Install dependencies:
```bash
go mod download
```

2. Run the server:
```bash
go run main.go
```

The server will start and listen on the configured port. All HTTP and WebSocket requests matching the configured paths will be forwarded to their respective target ports.

## Example

If you have the following configuration:
```yaml
routes:
  - path: "/api"
    port: 8081
  - path: "/ws"
    port: 8082
server:
  port: 8080
```

- HTTP requests to `http://localhost:8080/api/*` will be forwarded to `http://localhost:8081/*`
- WebSocket connections to `ws://localhost:8080/ws/*` will be forwarded to `ws://localhost:8082/*`

## Dependencies

- [gorilla/websocket](https://github.com/gorilla/websocket)
- [spf13/viper](https://github.com/spf13/viper) 