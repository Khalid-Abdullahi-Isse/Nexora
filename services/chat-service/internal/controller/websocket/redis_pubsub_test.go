package websocket

import (
	"context"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"os"
	"testing"
	"time"
)

func TestRedisRemotePodDelivery(t *testing.T) {
	addr := os.Getenv("CHAT_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("isolated Redis required")
	}
	r := redis.NewClient(&redis.Options{Addr: addr})
	defer r.Close()
	a, b := NewHub(4), NewHub(4)
	defer a.Close()
	defer b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	busA, e := NewBus(ctx, r, a)
	if e != nil {
		t.Fatal(e)
	}
	defer busA.Close()
	busB, e := NewBus(ctx, r, b)
	if e != nil {
		t.Fatal(e)
	}
	defer busB.Close()
	user := uuid.NewString()
	local, remote, outsider := testClient(user), testClient(user), testClient(uuid.NewString())
	a.Register(local)
	b.Register(remote)
	b.Register(outsider)
	defer a.Remove(local)
	defer b.Remove(remote)
	defer b.Remove(outsider)
	if e = busA.Publish(ctx, []string{user}, models.Event{Type: "message.created", Data: map[string]string{"conversationId": uuid.NewString()}}); e != nil {
		t.Fatal(e)
	}
	for _, c := range []*Client{local, remote} {
		select {
		case <-c.Send:
		case <-ctx.Done():
			t.Fatal("cross pod delivery timeout")
		}
	}
	select {
	case <-outsider.Send:
		t.Fatal("recipient leak")
	case <-time.After(50 * time.Millisecond):
	}
	select {
	case <-local.Send:
		t.Fatal("duplicate local event")
	default:
	}
}
