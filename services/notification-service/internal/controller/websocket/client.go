package websocket

import (
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/config"
	"github.com/gorilla/websocket"
	"sync"
	"time"
)

type Client struct {
	UserID string
	Conn   *websocket.Conn
	Send   chan []byte
	done   chan struct{}
	once   sync.Once
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
