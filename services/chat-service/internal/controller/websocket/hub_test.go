package websocket

import (
	"sync"
	"testing"
)

func testClient(u string) *Client {
	return &Client{UserID: u, Send: make(chan []byte, 8), done: make(chan struct{})}
}
func TestHubIsolationCleanupAndBackpressure(t *testing.T) {
	h := NewHub(2)
	a, b, c := testClient("a"), testClient("a"), testClient("b")
	for _, v := range []*Client{a, b, c} {
		if !h.Register(v) {
			t.Fatal("register")
		}
	}
	if h.Register(testClient("a")) {
		t.Fatal("connection bound")
	}
	h.Deliver([]string{"a"}, []byte(`{"type":"message.created","data":{"conversationId":"room"}}`))
	if len(a.Send) != 1 || len(b.Send) != 1 || len(c.Send) != 0 {
		t.Fatal("recipient isolation")
	}
	a.subscription("room", false)
	h.Deliver([]string{"a"}, []byte(`{"data":{"conversationId":"room"}}`))
	if len(a.Send) != 1 || len(b.Send) != 2 {
		t.Fatal("leave subscription")
	}
	for i := 0; i < 20; i++ {
		c.Enqueue([]byte("x"))
	}
	select {
	case <-c.done:
	default:
		t.Fatal("slow client not closed")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); h.Deliver([]string{"a"}, []byte(`{}`)) }()
	}
	wg.Wait()
	for _, v := range []*Client{a, b, c} {
		h.Remove(v)
		h.Remove(v)
	}
	h.Close()
	if len(h.clients) != 0 {
		t.Fatal("leaked clients")
	}
	if h.Register(testClient("x")) {
		t.Fatal("registered during shutdown")
	}
}
