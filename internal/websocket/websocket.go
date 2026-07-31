// Package websocket implements the minimal server side of the
// WebSocket protocol (RFC 6455) needed to push text messages to
// browsers: the upgrade handshake, text frames and the close frame.
package websocket

import (
	"bufio"
	// #nosec G505 -- RFC 6455 mandates SHA-1 for the handshake accept key.
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
)

// acceptGUID is the fixed GUID from RFC 6455 section 1.3 used to
// derive the Sec-WebSocket-Accept value.
const acceptGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

const (
	opText  = 0x1
	opClose = 0x8
	finBit  = 0x80
)

// ErrNotWebSocket is returned when the request is not a valid
// WebSocket upgrade request.
var ErrNotWebSocket = errors.New("not a websocket upgrade request")

// Conn is a server-side WebSocket connection.
type Conn struct {
	conn net.Conn
	bw   *bufio.Writer
	mu   sync.Mutex
}

// Upgrade hijacks the HTTP connection and completes the WebSocket
// opening handshake. On failure it writes an error response and
// returns ErrNotWebSocket.
func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if r.Method != http.MethodGet ||
		!headerContains(r.Header, "Connection", "upgrade") ||
		!strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
		r.Header.Get("Sec-WebSocket-Version") != "13" ||
		r.Header.Get("Sec-WebSocket-Key") == "" {
		http.Error(w, "bad websocket handshake", http.StatusBadRequest)
		return nil, ErrNotWebSocket
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
		return nil, ErrNotWebSocket
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	accept := computeAccept(r.Header.Get("Sec-WebSocket-Key"))
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := rw.WriteString(response); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &Conn{conn: conn, bw: rw.Writer}, nil
}

// WriteMessage sends payload as a single text frame.
func (c *Conn) WriteMessage(payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.writeFrame(finBit|opText, payload); err != nil {
		return err
	}
	return c.bw.Flush()
}

// Close sends a close frame and closes the underlying connection.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.writeFrame(finBit|opClose, nil); err == nil {
		_ = c.bw.Flush()
	}
	return c.conn.Close()
}

// RemoteAddr returns the remote address of the connection.
func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// writeFrame writes a single unmasked frame (servers never mask).
func (c *Conn) writeFrame(opcode byte, payload []byte) error {
	header := make([]byte, 0, 10)
	header = append(header, opcode)

	switch l := len(payload); {
	case l < 126:
		header = append(header, byte(l))
	case l < 1<<16:
		header = append(header, 126, 0, 0)
		binary.BigEndian.PutUint16(header[2:], uint16(l))
	default:
		header = append(header, 127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[2:], uint64(l))
	}

	if _, err := c.bw.Write(header); err != nil {
		return err
	}
	_, err := c.bw.Write(payload)
	return err
}

// computeAccept derives the Sec-WebSocket-Accept value for a client
// key as required by RFC 6455.
func computeAccept(key string) string {
	// #nosec G401 -- RFC 6455 mandates SHA-1 here; it is not used for security.
	sum := sha1.Sum([]byte(key + acceptGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// headerContains reports whether a comma-separated header contains the
// given token, ignoring case.
func headerContains(h http.Header, name, token string) bool {
	for _, value := range h.Values(name) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}
