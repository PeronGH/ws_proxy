package server

import (
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

// Client represents a single connected WebSocket proxy client.
type Client struct {
	manager *Manager
	conn    *websocket.Conn
	send    chan []byte
	id      string
}

// readPump pumps messages from the websocket connection to the manager.
func (c *Client) readPump() {
	defer func() {
		c.manager.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		c.manager.handleIncomingMessage(message)
	}
}

// writePump pumps messages from the send channel to the websocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The manager closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Manager handles all connected clients and proxy requests.
type Manager struct {
	clients     map[*Client]bool
	clientIndex []*Client
	nextClient  int
	register    chan *Client
	unregister  chan *Client
	clientMutex sync.RWMutex

	pending      map[string]chan ProxyMessageUnion
	pendingMutex sync.RWMutex
}

// NewManager creates a new Manager instance.
func NewManager() *Manager {
	return &Manager{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		pending:    make(map[string]chan ProxyMessageUnion),
	}
}

// Run starts the manager's event loop.
func (m *Manager) Run() {
	for {
		select {
		case client := <-m.register:
			m.clientMutex.Lock()
			m.clients[client] = true
			m.updateClientIndex()
			log.Printf("Client %s connected. Total clients: %d", client.id, len(m.clients))
			m.clientMutex.Unlock()
		case client := <-m.unregister:
			m.clientMutex.Lock()
			if _, ok := m.clients[client]; ok {
				delete(m.clients, client)
				close(client.send)
				m.updateClientIndex()
				log.Printf("Client %s disconnected. Total clients: %d", client.id, len(m.clients))
			}
			m.clientMutex.Unlock()
		}
	}
}

func (m *Manager) updateClientIndex() {
	m.clientIndex = make([]*Client, 0, len(m.clients))
	for c := range m.clients {
		m.clientIndex = append(m.clientIndex, c)
	}
}

// getNextClient selects a client using round-robin.
func (m *Manager) getNextClient() (*Client, error) {
	m.clientMutex.RLock()
	defer m.clientMutex.RUnlock()

	if len(m.clientIndex) == 0 {
		return nil, errors.New("no available proxy clients")
	}
	m.nextClient = (m.nextClient + 1) % len(m.clientIndex)
	return m.clientIndex[m.nextClient], nil
}

// registerPendingRequest creates a channel to wait for a response for a given UUID.
func (m *Manager) registerPendingRequest(uuid string) <-chan ProxyMessageUnion {
	m.pendingMutex.Lock()
	defer m.pendingMutex.Unlock()
	ch := make(chan ProxyMessageUnion, 2) // Buffer to avoid blocking on headers+chunk
	m.pending[uuid] = ch
	return ch
}

// unregisterPendingRequest cleans up the pending request channel.
func (m *Manager) unregisterPendingRequest(uuid string) {
	m.pendingMutex.Lock()
	defer m.pendingMutex.Unlock()
	if ch, ok := m.pending[uuid]; ok {
		close(ch)
		delete(m.pending, uuid)
	}
}

// handleIncomingMessage routes a message from a client to the correct pending request channel.
func (m *Manager) handleIncomingMessage(message []byte) {
	var base ProxyMessageBase
	if err := json.Unmarshal(message, &base); err != nil {
		log.Printf("Could not unmarshal base message: %v", err)
		return
	}

	m.pendingMutex.RLock()
	ch, ok := m.pending[base.UUID]
	m.pendingMutex.RUnlock()

	if !ok {
		log.Printf("Received message for unknown UUID: %s", base.UUID)
		return
	}

	var msg ProxyMessageUnion
	switch base.Type {
	case "response-headers":
		var headersMsg ProxyResponseHeaders
		json.Unmarshal(message, &headersMsg)
		msg = headersMsg
	case "response-chunk":
		var chunkMsg ProxyResponseChunk
		json.Unmarshal(message, &chunkMsg)
		msg = chunkMsg
	default:
		log.Printf("Unknown message type received: %s", base.Type)
		return
	}

	// Send message to the waiting handler
	ch <- msg
}
