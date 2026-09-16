package websocket

import (
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/config"
	"github.com/gorilla/websocket"
	"sync"
	"time"
)

type Client struct {
	subscriptionMu sync.RWMutex
	muted          map[string]bool
	UserID         string
	Conn           *websocket.Conn
	Send           chan []byte
	done           chan struct{}
	once           sync.Once
}

func (c *Client) Close() { c.once.Do(func() { close(c.done) }) }
func (c *Client) Enqueue(b []byte) {
	select {
	case <-c.done:
		return
	default:
	}
	select {
	case c.Send <- b:
	default:
		c.Close()
	}
}
func (c *Client) write(cfg config.Realtime) {
	defer c.Close()
	defer c.Conn.Close()
	tick := time.NewTicker(cfg.PingInterval)
	defer tick.Stop()
	for {
		select {
		case <-c.done:
			_ = c.Conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "Reconnect and sync history"), time.Now().Add(cfg.WriteTimeout))
			return
		case b := <-c.Send:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
			if c.Conn.WriteMessage(websocket.TextMessage, b) != nil {
				return
			}
		case <-tick.C:
			if c.Conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(cfg.WriteTimeout)) != nil {
				return
			}
		}
	}
}

func (c *Client) subscription(id string, join bool) bool {
	c.subscriptionMu.Lock()
	defer c.subscriptionMu.Unlock()
	if join {
		delete(c.muted, id)
		return true
	}
	if len(c.muted) >= 256 {
		return false
	}
	if c.muted == nil {
		c.muted = map[string]bool{}
	}
	c.muted[id] = true
	return true
}
func (c *Client) accepts(id string) bool {
	c.subscriptionMu.RLock()
	defer c.subscriptionMu.RUnlock()
	return !c.muted[id]
}