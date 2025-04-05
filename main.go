package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"fmt"

	"github.com/gorilla/websocket"
	"github.com/spf13/viper"
)

type Config struct {
	Routes []struct {
		Path string `mapstructure:"path"`
		Port int    `mapstructure:"port"`
	} `mapstructure:"routes"`
	Server struct {
		Port int `mapstructure:"port"`
	} `mapstructure:"server"`
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

	// Create route handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Check if it's a WebSocket request
		if websocket.IsWebSocketUpgrade(r) {
			handleWebSocket(w, r, config)
			return
		}

		// Handle HTTP request
		handleHTTP(w, r, config)
	})

	// Start server
	log.Printf("Server starting on port %d", config.Server.Port)
	port := fmt.Sprintf(":%d", config.Server.Port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func handleHTTP(w http.ResponseWriter, r *http.Request, config Config) {
	for _, route := range config.Routes {
		if strings.HasPrefix(r.URL.Path, route.Path) {
			path := fmt.Sprintf("http://localhost:%d", route.Port)
			targetURL, err := url.Parse(path)
			if err != nil {
				log.Printf("Failed to parse target URL: %v", err)
				http.Error(w, "Failed to parse target URL", http.StatusInternalServerError)
				return
			}
			proxy := httputil.NewSingleHostReverseProxy(targetURL)
			r.URL.Path = strings.TrimPrefix(r.URL.Path, route.Path)
			proxy.ServeHTTP(w, r)
			return
		}
	}
	http.NotFound(w, r)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request, config Config) {
	for _, route := range config.Routes {
		if strings.HasPrefix(r.URL.Path, route.Path) {
			// Establish WebSocket connection with target server
			path := fmt.Sprintf("ws://localhost:%d", route.Port)
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
