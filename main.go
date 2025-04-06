package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/viper"
)

// ANSI color codes for terminal
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
)

type RedirectConfig struct {
	Path string `mapstructure:"path"`
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type ServerConfig struct {
	Server   int              `mapstructure:"server"`
	Redirect []RedirectConfig `mapstructure:"redirect"`
}

type Config struct {
	Router []ServerConfig `mapstructure:"router"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func main() {
	// Configure viper
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	// Read configuration file
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	// Setup signal catching
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	var wg sync.WaitGroup
	// Channel to collect all server instances for graceful shutdown
	servers := make([]*http.Server, 0, len(config.Router))
	serversMutex := sync.Mutex{}

	// Start a server for each server configuration
	for _, serverConfig := range config.Router {
		wg.Add(1)
		// Use goroutine to start each server
		go func(serverCfg ServerConfig) {
			defer wg.Done()

			// Create route handler
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				// Check if it's a WebSocket request
				if websocket.IsWebSocketUpgrade(r) {
					handleWebSocket(w, r, serverCfg.Redirect)
					return
				}

				// Handle HTTP request
				handleHTTP(w, r, serverCfg.Redirect)
			})

			// Configure server with proper shutdown
			addr := fmt.Sprintf(":%d", serverCfg.Server)
			srv := &http.Server{
				Addr:    addr,
				Handler: mux,
			}

			// Add server to the list for shutdown
			serversMutex.Lock()
			servers = append(servers, srv)
			serversMutex.Unlock()

			// Log server routes
			writer := strings.Builder{}
			writer.WriteString(fmt.Sprintf("%sServer starting on port %s%d%s with the following routes:",
				ColorGreen, ColorCyan, serverCfg.Server, ColorReset))
			for _, route := range serverCfg.Redirect {
				host := route.Host
				if len(host) == 0 {
					host = "localhost"
				}
				writer.WriteString(fmt.Sprintf("\n\t%s%s%s -> %s%s:%d%s",
					ColorYellow, route.Path, ColorReset,
					ColorGreen, host, route.Port, ColorReset))
			}
			log.Print(writer.String())

			// Start server
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("%sFailed to start server on port %d: %v%s", ColorRed, serverCfg.Server, err, ColorReset)
			}
			log.Printf("%sServer on port %d has been shutdown%s",
				ColorYellow, serverCfg.Server, ColorReset)
		}(serverConfig)
	}

	// Wait for interrupt signal
	<-stop
	log.Println("Received shutdown signal, gracefully shutting down...")

	// Create a timeout context for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown all servers
	shutdownWg := sync.WaitGroup{}
	serversMutex.Lock()
	for _, srv := range servers {
		shutdownWg.Add(1)
		go func(s *http.Server) {
			defer shutdownWg.Done()

			if err := s.Shutdown(ctx); err != nil {
				log.Printf("Error during server shutdown: %v", err)
			}
		}(srv)
	}
	serversMutex.Unlock()

	// Wait for all servers to complete graceful shutdown
	shutdownChan := make(chan struct{})
	go func() {
		shutdownWg.Wait()
		close(shutdownChan)
	}()

	// Wait for either context timeout or all servers to shutdown
	select {
	case <-ctx.Done():
		log.Println("Shutdown timed out, forcing exit")
	case <-shutdownChan:
		log.Println("All servers gracefully shut down")
	}
}

func handleHTTP(w http.ResponseWriter, r *http.Request, routes []RedirectConfig) {
	log.Printf("%sReceived request: %s%s", ColorYellow, r.URL.Path, ColorReset)

	for _, route := range routes {
		if strings.HasPrefix(r.URL.Path, route.Path) {
			host := route.Host
			if len(host) == 0 {
				host = "localhost"
			}

			// Log routing match
			log.Printf("%sMatched route: %s -> %s:%d%s", ColorGreen, route.Path, host, route.Port, ColorReset)

			// Build URL
			targetURL, err := url.Parse(fmt.Sprintf("http://%s:%d", host, route.Port))
			if err != nil {
				log.Printf("%sFailed to parse target URL: %v%s", ColorRed, err, ColorReset)
				http.Error(w, "Failed to parse target URL", http.StatusInternalServerError)
				return
			}

			// Create and configure reverse proxy
			proxy := httputil.NewSingleHostReverseProxy(targetURL)

			// Modify default Director function
			originalDirector := proxy.Director
			proxy.Director = func(req *http.Request) {
				originalDirector(req)

				// Preserve original request path
				req.URL.Path = r.URL.Path
				if r.URL.RawQuery != "" {
					req.URL.RawQuery = r.URL.RawQuery
				}

				// Set X-Forwarded headers
				req.Header.Set("X-Forwarded-Host", req.Host)
				req.Header.Set("X-Forwarded-Proto", "http")
				req.Header.Set("X-Forwarded-For", r.RemoteAddr)

				// Log complete forwarding URL
				log.Printf("%sForwarding request to: %s%s", ColorCyan, req.URL.String(), ColorReset)
			}

			// Add error handling
			proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
				log.Printf("%sProxy error: %v%s", ColorRed, err, ColorReset)
				http.Error(rw, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
			}

			proxy.ServeHTTP(w, r)
			return
		}
	}

	log.Printf("%sNo matching route found: %s%s", ColorRed, r.URL.Path, ColorReset)
	http.NotFound(w, r)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request, routes []RedirectConfig) {
	log.Printf("%sReceived WebSocket request: %s%s", ColorYellow, r.URL.Path, ColorReset)

	for _, route := range routes {
		if strings.HasPrefix(r.URL.Path, route.Path) {
			// Establish WebSocket connection with target server
			host := route.Host
			if len(host) == 0 {
				host = "localhost"
			}

			// Log routing target
			log.Printf("%sMatched WebSocket route: %s -> %s:%d%s", ColorGreen, route.Path, host, route.Port, ColorReset)

			// Build WebSocket URL
			wsURL := fmt.Sprintf("ws://%s:%d%s", host, route.Port, r.URL.Path)
			log.Printf("%sAttempting WebSocket connection: %s%s", ColorCyan, wsURL, ColorReset)

			targetConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				log.Printf("%sWebSocket server connection failed: %v%s", ColorRed, err, ColorReset)
				http.Error(w, "Failed to connect to target server", http.StatusInternalServerError)
				return
			}
			defer targetConn.Close()
			log.Printf("%sWebSocket connection established successfully%s", ColorGreen, ColorReset)

			// Upgrade client connection
			clientConn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Printf("%sWebSocket upgrade failed: %v%s", ColorRed, err, ColorReset)
				http.Error(w, "Failed to upgrade WebSocket connection", http.StatusInternalServerError)
				return
			}
			defer clientConn.Close()
			log.Printf("%sClient WebSocket upgrade successful%s", ColorGreen, ColorReset)

			// Forward messages
			go func() {
				for {
					messageType, message, err := clientConn.ReadMessage()
					if err != nil {
						log.Printf("%sRead from client failed: %v%s", ColorRed, err, ColorReset)
						break
					}
					if err := targetConn.WriteMessage(messageType, message); err != nil {
						log.Printf("%sWrite to server failed: %v%s", ColorRed, err, ColorReset)
						break
					}
				}
			}()

			for {
				messageType, message, err := targetConn.ReadMessage()
				if err != nil {
					log.Printf("%sRead from server failed: %v%s", ColorRed, err, ColorReset)
					break
				}
				if err := clientConn.WriteMessage(messageType, message); err != nil {
					log.Printf("%sWrite to client failed: %v%s", ColorRed, err, ColorReset)
					break
				}
			}
			return
		}
	}

	log.Printf("%sNo matching WebSocket route found: %s%s", ColorRed, r.URL.Path, ColorReset)
	http.NotFound(w, r)
}
