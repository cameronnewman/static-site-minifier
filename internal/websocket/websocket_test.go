package websocket

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const sampleKey = "dGhlIHNhbXBsZSBub25jZQ=="

// sampleAccept is the accept value for sampleKey from RFC 6455 1.3.
const sampleAccept = "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="

func TestComputeAccept(t *testing.T) {
	if got := computeAccept(sampleKey); got != sampleAccept {
		t.Fatalf("computeAccept = %q, want %q", got, sampleAccept)
	}
}

// upgradeServer returns a test server that upgrades connections and a
// way to retrieve the server-side Conn.
func upgradeServer(t *testing.T) (*httptest.Server, func() *Conn) {
	t.Helper()

	var mu sync.Mutex
	var serverConn *Conn

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			return
		}
		mu.Lock()
		serverConn = conn
		mu.Unlock()
	}))
	t.Cleanup(ts.Close)

	await := func() *Conn {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			c := serverConn
			mu.Unlock()
			if c != nil {
				return c
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("server connection never established")
		return nil
	}
	return ts, await
}

// dial performs the client half of the handshake and returns the raw
// connection and the response headers.
func dial(t *testing.T, ts *httptest.Server) (net.Conn, map[string]string) {
	t.Helper()
	addr := strings.TrimPrefix(ts.URL, "http://")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	request := "GET / HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: keep-alive, Upgrade\r\n" +
		"Sec-WebSocket-Key: " + sampleKey + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(conn)
	status, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("status line = %q, want 101", status)
	}

	headers := make(map[string]string)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
		if name, value, ok := strings.Cut(strings.TrimRight(line, "\r\n"), ": "); ok {
			headers[strings.ToLower(name)] = value
		}
	}
	return conn, headers
}

// readFrame reads one frame from the connection.
func readFrame(t *testing.T, conn net.Conn) (opcode byte, payload []byte) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}

	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		t.Fatal(err)
	}
	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		ext := make([]byte, 2)
		if _, err := io.ReadFull(conn, ext); err != nil {
			t.Fatal(err)
		}
		length = uint64(binary.BigEndian.Uint16(ext))
	case 127:
		ext := make([]byte, 8)
		if _, err := io.ReadFull(conn, ext); err != nil {
			t.Fatal(err)
		}
		length = binary.BigEndian.Uint64(ext)
	}

	payload = make([]byte, length)
	if _, err := io.ReadFull(conn, payload); err != nil {
		t.Fatal(err)
	}
	return header[0], payload
}

func TestUpgradeAndWriteMessage(t *testing.T) {
	ts, await := upgradeServer(t)
	conn, headers := dial(t, ts)

	if got := headers["sec-websocket-accept"]; got != sampleAccept {
		t.Fatalf("accept header = %q, want %q", got, sampleAccept)
	}

	server := await()
	if server.RemoteAddr() == nil {
		t.Error("RemoteAddr returned nil")
	}

	if err := server.WriteMessage([]byte("reload")); err != nil {
		t.Fatal(err)
	}
	opcode, payload := readFrame(t, conn)
	if opcode != finBit|opText {
		t.Fatalf("opcode = %#x, want %#x", opcode, finBit|opText)
	}
	if string(payload) != "reload" {
		t.Fatalf("payload = %q, want %q", payload, "reload")
	}
}

func TestWriteMessageMediumFrame(t *testing.T) {
	ts, await := upgradeServer(t)
	conn, _ := dial(t, ts)
	server := await()

	message := strings.Repeat("x", 300)
	if err := server.WriteMessage([]byte(message)); err != nil {
		t.Fatal(err)
	}
	if _, payload := readFrame(t, conn); string(payload) != message {
		t.Fatalf("payload length = %d, want %d", len(payload), len(message))
	}
}

func TestWriteMessageLargeFrame(t *testing.T) {
	ts, await := upgradeServer(t)
	conn, _ := dial(t, ts)
	server := await()

	message := strings.Repeat("y", 70000)
	if err := server.WriteMessage([]byte(message)); err != nil {
		t.Fatal(err)
	}
	if _, payload := readFrame(t, conn); string(payload) != message {
		t.Fatalf("payload length = %d, want %d", len(payload), len(message))
	}
}

func TestClose(t *testing.T) {
	ts, await := upgradeServer(t)
	conn, _ := dial(t, ts)
	server := await()

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	opcode, _ := readFrame(t, conn)
	if opcode != finBit|opClose {
		t.Fatalf("opcode = %#x, want close frame %#x", opcode, finBit|opClose)
	}
}

func TestWriteAfterCloseFails(t *testing.T) {
	ts, await := upgradeServer(t)
	_, _ = dial(t, ts)
	server := await()

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if err := server.WriteMessage([]byte("reload")); err == nil {
		t.Fatal("expected write on closed connection to fail")
	}
}

func TestUpgradeRejectsBadRequests(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := Upgrade(w, r); err == nil {
			t.Error("expected upgrade error")
		}
	}))
	defer ts.Close()

	tests := []struct {
		name    string
		headers map[string]string
		method  string
	}{
		{name: "plain GET", method: http.MethodGet},
		{name: "POST", method: http.MethodPost, headers: map[string]string{
			"Upgrade": "websocket", "Connection": "Upgrade",
			"Sec-WebSocket-Key": sampleKey, "Sec-WebSocket-Version": "13",
		}},
		{name: "wrong version", method: http.MethodGet, headers: map[string]string{
			"Upgrade": "websocket", "Connection": "Upgrade",
			"Sec-WebSocket-Key": sampleKey, "Sec-WebSocket-Version": "8",
		}},
		{name: "missing key", method: http.MethodGet, headers: map[string]string{
			"Upgrade": "websocket", "Connection": "Upgrade",
			"Sec-WebSocket-Version": "13",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, ts.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			for name, value := range tt.headers {
				req.Header.Set(name, value)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Error(err)
				}
			}()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}
