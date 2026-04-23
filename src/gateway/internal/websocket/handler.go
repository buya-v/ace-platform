package websocket

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-5AB5380DC63B"

// Message is the WebSocket message envelope sent to clients.
type Message struct {
	Type         string          `json:"type"`
	InstrumentID string          `json:"instrument_id,omitempty"`
	Sequence     int64           `json:"sequence,omitempty"`
	Timestamp    string          `json:"timestamp"`
	Data         json.RawMessage `json:"data,omitempty"`
}

// Handler manages WebSocket connections.
type Handler struct {
	heartbeatInterval time.Duration
}

// NewHandler creates a new WebSocket handler.
func NewHandler() *Handler {
	return &Handler{
		heartbeatInterval: 30 * time.Second,
	}
}

// TradesHandler handles WebSocket upgrade for /api/v1/ws/trades/{instrument_id}.
func (h *Handler) TradesHandler(w http.ResponseWriter, r *http.Request) {
	instrumentID := r.URL.Query().Get("instrument_id")
	conn, err := h.upgrade(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	log.Printf("WebSocket connected: trades stream for %s", instrumentID)
	h.streamHeartbeats(conn, "trade", instrumentID)
}

// BookHandler handles WebSocket upgrade for /api/v1/ws/book/{instrument_id}.
func (h *Handler) BookHandler(w http.ResponseWriter, r *http.Request) {
	instrumentID := r.URL.Query().Get("instrument_id")
	conn, err := h.upgrade(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	log.Printf("WebSocket connected: book stream for %s", instrumentID)
	h.streamHeartbeats(conn, "book_update", instrumentID)
}

// ExecutionsHandler handles WebSocket upgrade for /api/v1/ws/executions.
func (h *Handler) ExecutionsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrade(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	log.Printf("WebSocket connected: executions stream")
	h.streamHeartbeats(conn, "execution_report", "")
}

// upgrade performs the WebSocket handshake using raw HTTP hijacking (zero deps).
func (h *Handler) upgrade(w http.ResponseWriter, r *http.Request) (net.Conn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
		!strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		http.Error(w, "Expected WebSocket upgrade", http.StatusBadRequest)
		return nil, http.ErrNotSupported
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "Missing Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, http.ErrNotSupported
	}

	// Compute accept key
	hash := sha1.New()
	hash.Write([]byte(key + websocketGUID))
	acceptKey := base64.StdEncoding.EncodeToString(hash.Sum(nil))

	// Hijack the connection
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket upgrade not supported", http.StatusInternalServerError)
		return nil, http.ErrNotSupported
	}

	conn, buf, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	// Write upgrade response
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n"
	buf.WriteString(resp)
	buf.Flush()

	return conn, nil
}

// streamHeartbeats sends periodic heartbeat frames until the connection closes.
// In production, this would also forward real gRPC stream messages.
func (h *Handler) streamHeartbeats(conn net.Conn, msgType, instrumentID string) {
	ticker := time.NewTicker(h.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			msg := Message{
				Type:         "heartbeat",
				InstrumentID: instrumentID,
				Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
			}
			data, _ := json.Marshal(msg)
			if err := writeWSTextFrame(conn, data); err != nil {
				return
			}
		}
	}
}

// writeWSTextFrame writes a WebSocket text frame (opcode 0x81).
func writeWSTextFrame(conn net.Conn, payload []byte) error {
	w := bufio.NewWriter(conn)
	length := len(payload)

	// First byte: FIN + text opcode
	w.WriteByte(0x81)

	// Length encoding (server→client, no mask)
	if length <= 125 {
		w.WriteByte(byte(length))
	} else if length <= 65535 {
		w.WriteByte(126)
		w.WriteByte(byte(length >> 8))
		w.WriteByte(byte(length))
	} else {
		w.WriteByte(127)
		for i := 7; i >= 0; i-- {
			w.WriteByte(byte(length >> (8 * i)))
		}
	}

	w.Write(payload)
	return w.Flush()
}
