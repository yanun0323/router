package main

import (
	"context"
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

	"fmt"

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
			writer.WriteString(fmt.Sprintf("Server starting on port %s%d%s with the following routes:",
				ColorGreen, serverCfg.Server, ColorReset))
			for _, route := range serverCfg.Redirect {
				writer.WriteString(fmt.Sprintf("\n\t%s%s%s -> %s%d%s",
					ColorCyan, route.Path, ColorReset,
					ColorYellow, route.Port, ColorReset))
			}
			log.Print(writer.String())

			// Start server
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start server on port %d: %v", serverCfg.Server, err)
			}
			log.Printf("Server on port %s%d%s has been shutdown",
				ColorGreen, serverCfg.Server, ColorReset)
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
	for _, route := range routes {
		if strings.HasPrefix(r.URL.Path, route.Path) {
			path := fmt.Sprintf("http://localhost:%d", route.Port)
			targetURL, err := url.Parse(path)
			if err != nil {
				log.Printf("Failed to parse target URL: %v", err)
				http.Error(w, "Failed to parse target URL", http.StatusInternalServerError)
				return
			}
			proxy := httputil.NewSingleHostReverseProxy(targetURL)
			proxy.ServeHTTP(w, r)
			return
		}
	}
	http.NotFound(w, r)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request, routes []RedirectConfig) {
	for _, route := range routes {
		if strings.HasPrefix(r.URL.Path, route.Path) {
			// Establish WebSocket connection with target server
			path := fmt.Sprintf("ws://localhost:%d%s", route.Port, r.URL.Path)
			targetURL, err := url.Parse(path)
			if err != nil {
				log.Printf("Failed to parse target URL: %v", err)
				http.Error(w, "Failed to parse target URL", http.StatusInternalServerError)
				return
			}
			targetConn, _, err := websocket.DefaultDialer.Dial(targetURL.String(), nil)
			if err != nil {
				log.Printf("Failed to connect to target server: %v", err)
				http.Error(w, "Failed to connect to target server", http.StatusInternalServerError)
				return
			}
			defer targetConn.Close()

			// Upgrade client connection
			clientConn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Printf("Failed to upgrade WebSocket connection: %v", err)
				http.Error(w, "Failed to upgrade WebSocket connection", http.StatusInternalServerError)
				return
			}
			defer clientConn.Close()

			// Forward messages
			go func() {
				for {
					messageType, message, err := clientConn.ReadMessage()
					if err != nil {
						break
					}
					if err := targetConn.WriteMessage(messageType, message); err != nil {
						break
					}
				}
			}()

			for {
				messageType, message, err := targetConn.ReadMessage()
				if err != nil {
					break
				}
				if err := clientConn.WriteMessage(messageType, message); err != nil {
					break
				}
			}
			return
		}
	}
	http.NotFound(w, r)
}
