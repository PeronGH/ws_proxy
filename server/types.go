package server

// ProxyMessageBase is the base for all messages, containing the type and a unique ID.
type ProxyMessageBase struct {
	Type string `json:"type"`
	UUID string `json:"uuid"`
}

// ProxyRequest is sent from the server to the client, asking it to make an HTTP request.
type ProxyRequest struct {
	ProxyMessageBase
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body,omitempty"`
}

// ProxyResponseHeaders is sent from the client to the server with the initial response details.
type ProxyResponseHeaders struct {
	ProxyMessageBase
	Status     int               `json:"status"`
	StatusText string            `json:"statusText"`
	Headers    map[string]string `json:"headers"`
}

// ProxyResponseChunk is a piece of the response body sent from the client to the server.
type ProxyResponseChunk struct {
	ProxyMessageBase
	Data    string `json:"data"`
	IsFinal bool   `json:"isFinal"`
}

// ProxyMessageUnion is used for unmarshaling to determine the message type.
// We don't use this directly but it's good practice to conceptualize it.
// The actual logic will unmarshal into ProxyMessageBase first.
type ProxyMessageUnion interface{}
