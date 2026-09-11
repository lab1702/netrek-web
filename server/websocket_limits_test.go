package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Exercise the actual compressed reader and readPump without starting a game loop.
func readPumpPeer(t *testing.T) (*Server, *Client, *websocket.Conn) {
	t.Helper()
	s := NewServer()
	s.unregister = make(chan *Client, 1)
	clients := make(chan *Client, 1)
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		c := &Client{ID: 1, server: s, conn: conn, send: make(chan ServerMessage, 64)}
		c.SetPlayerID(-1)
		clients <- c
		c.readPump()
	}))
	dialer := websocket.Dialer{EnableCompression: true}
	peer, response, err := dialer.Dial("ws"+h.URL[4:], nil)
	if err != nil {
		h.Close()
		t.Fatal(err)
	}
	if response.Header.Get("Sec-WebSocket-Extensions") == "" {
		t.Fatal("test did not negotiate compression")
	}
	c := <-clients
	t.Cleanup(func() { peer.Close(); c.conn.Close(); h.Close() })
	return s, c, peer
}

func TestCompressedMessagesRespectDecodedSizeLimit(t *testing.T) {
	for _, size := range []int{maxClientMessageBytes, maxClientMessageBytes + 1, 1_000_000} {
		s, c, peer := readPumpPeer(t)
		message := `{"type":"login","data":{"name":"Pilot","team":1,"ship":2},"padding":""}`
		message = strings.Replace(message, `"padding":""`, `"padding":"`+strings.Repeat("x", size-len(message))+`"`, 1)
		if err := peer.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
			t.Fatal(err)
		}
		if size <= maxClientMessageBytes {
			select {
			case msg := <-c.send:
				if msg.Type != "login_success" {
					t.Fatalf("valid compressed message rejected: %+v", msg)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("valid compressed message was not processed")
			}
			peer.Close()
		}
		select {
		case <-s.unregister:
		case <-time.After(2 * time.Second):
			t.Fatalf("reader did not close for decoded size %d", size)
		}
		if size > maxClientMessageBytes && c.GetPlayerID() != -1 {
			t.Fatal("oversized compressed login was decoded and accepted")
		}
	}
}

func TestReadPumpDisconnectsExcessiveSenders(t *testing.T) {
	s, _, peer := readPumpPeer(t)
	for i := 0; i < 51; i++ {
		err := peer.WriteJSON(ClientMessage{Type: MsgTypeShields, Data: json.RawMessage(`{}`)})
		if err != nil {
			break // The server may already have closed the peer.
		}
	}
	select {
	case <-s.unregister:
	case <-time.After(2 * time.Second):
		t.Fatal("excessive sender was not disconnected")
	}
}
