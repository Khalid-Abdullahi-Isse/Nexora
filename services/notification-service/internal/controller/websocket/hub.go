package websocket

import (
	"context"
	"encoding/json"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/models"
	"sync"
)

type Hub struct {
	mu             sync.Mutex
	clients        map[string]map[*Client]bool
	closed         bool
	wg             sync.WaitGroup
	MaxConnections int
}

func NewHub(max int) *Hub { return &Hub{clients: map[string]map[*Client]bool{}, MaxConnections: max} }
func (h *Hub) Register(c *Client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.clients[c.UserID]) >= h.MaxConnections {
		return false
	}
	if h.clients[c.UserID] == nil {
		h.clients[c.UserID] = map[*Client]bool{}
	}
	h.clients[c.UserID][c] = true
	h.wg.Add(1)
	return true
}
func (h *Hub) Remove(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.UserID][c] {
		delete(h.clients[c.UserID], c)
		if len(h.clients[c.UserID]) == 0 {
			delete(h.clients, c.UserID)
		}
		h.wg.Done()
	}
	c.Close()
}
func (h *Hub) Deliver(user string, b []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients[user] {
		c.Enqueue(b)
	}
}
func (h *Hub) Publish(_ context.Context, n models.Item) error {
	b, err := json.Marshal(models.Realtime{Type: "notification", Data: n})
	if err == nil {
		h.Deliver(n.RecipientID.String(), b)
	}
	return err
}
func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	for _, cs := range h.clients {
		for c := range cs {
			c.Close()
		}
	}
	h.mu.Unlock()
	h.wg.Wait()
}
