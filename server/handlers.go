package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true }, // Allow all origins
}

// MakeProxyHandler creates the main HTTP handler that forwards requests to a client.
func MakeProxyHandler(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, err := m.getNextClient()
		if err != nil {
			log.Println(err)
			http.Error(w, "No available proxy clients", http.StatusServiceUnavailable)
			return
		}

		log.Printf("Proxying request %s %s via client %s", r.Method, r.URL.Path, client.id)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()

		reqUUID := uuid.New().String()
		proxyReq := ProxyRequest{
			ProxyMessageBase: ProxyMessageBase{Type: "request", UUID: reqUUID},
			Method:           r.Method,
			Path:             r.URL.RequestURI(),
			Body:             string(body),
		}

		reqBytes, err := json.Marshal(proxyReq)
		if err != nil {
			http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
			return
		}

		// Register to receive the response and defer cleanup
		responseChan := m.registerPendingRequest(reqUUID)
		defer m.unregisterPendingRequest(reqUUID)

		// Send the request to the client
		client.send <- reqBytes

		// Wait for the response and stream it back
		timeout := time.After(30 * time.Second) // Overall request timeout
		headersReceived := false

		for {
			select {
			case msg, ok := <-responseChan:
				if !ok {
					// Channel closed, likely by unregister
					return
				}
				switch m := msg.(type) {
				case ProxyResponseHeaders:
					headersReceived = true
					for key, val := range m.Headers {
						w.Header().Set(key, val)
					}
					w.WriteHeader(m.Status)
				case ProxyResponseChunk:
					if !headersReceived {
						log.Printf("Error: Received chunk before headers for %s", reqUUID)
						http.Error(w, "Proxy protocol error", http.StatusInternalServerError)
						return
					}
					io.WriteString(w, m.Data)
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
					if m.IsFinal {
						return // Request is complete
					}
				}
			case <-timeout:
				log.Printf("Request %s timed out", reqUUID)
				http.Error(w, "Proxy request timed out", http.StatusGatewayTimeout)
				return
			}
		}
	}
}

// MakeWebSocketHandler creates the handler for the WebSocket connection endpoint.
func MakeWebSocketHandler(m *Manager, password string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check password if one is set
		if password != "" {
			queryPassword := r.URL.Query().Get("password")
			if queryPassword != password {
				log.Println("WebSocket connection rejected: invalid password")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Failed to upgrade connection:", err)
			return
		}

		client := &Client{
			manager: m,
			conn:    conn,
			send:    make(chan []byte, 256),
			id:      uuid.New().String(),
		}
		client.manager.register <- client

		// Start the read and write pumps in separate goroutines
		go client.writePump()
		go client.readPump()
	}
}
