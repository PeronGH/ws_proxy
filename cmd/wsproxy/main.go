package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"wsproxy/server"
)

func main() {
	port := flag.Int("port", 7769, "Port to listen on")
	password := flag.String("password", "", "Optional password to protect the WebSocket endpoint")
	flag.Parse()

	manager := server.NewManager()
	go manager.Run()

	// The WebSocket handler for proxy clients to connect to
	http.HandleFunc("/__ws_proxy", server.MakeWebSocketHandler(manager, *password))

	// The handler for all other requests, which will be proxied
	http.HandleFunc("/", server.MakeProxyHandler(manager))

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting WebSocket proxy server on %s", addr)
	log.Println("Proxy clients should connect to ws://<host>:<port>/__ws_proxy")
	if *password != "" {
		log.Println("Password protection is ENABLED")
	} else {
		log.Println("Password protection is DISABLED")
	}

	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
