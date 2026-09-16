package service

import (
	"context"
	"errors"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/google/uuid"
	"strings"
	"testing"
)

type fakeDB struct {
	Database
	saved []models.Message
	fail  bool
}

func (d *fakeDB) Send(_ context.Context, p authn.Principal, id, content string) (models.Message, error) {
	if d.fail {
		return models.Message{}, errors.New("unavailable")
	}
	m := models.Message{ID: uuid.New(), ConversationID: uuid.MustParse(id), SenderUserID: uuid.MustParse(p.UserID), Content: content}
	d.saved = append(d.saved, m)
	return m, nil
}
func (d *fakeDB) Recipients(context.Context, authn.Principal, string) ([]string, error) {
	return []string{"recipient"}, nil
}

type failBus struct {
	calls int
	db    *fakeDB
	t     *testing.T
}

func (b *failBus) Publish(context.Context, []string, models.Event) error {
	b.calls++
	if len(b.db.saved) == 0 {
		b.t.Fatal("published before persistence")
	}
	return errors.New("redis unavailable")
}
func TestSendDurabilityAndValidation(t *testing.T) {
	db := &fakeDB{}
	bus := &failBus{db: db, t: t}
	s := New(db, bus)
	p := authn.Principal{UserID: uuid.NewString()}
	id := uuid.NewString()
	for _, text := range []string{"", " \n ", strings.Repeat("a", 4097), "a\x00b", string([]byte{0xff})} {
		if _, e := s.Send(context.Background(), p, id, text, ""); e == nil {
			t.Fatalf("accepted invalid text")
		}
	}
	m, e := s.Send(context.Background(), p, id, " hello ", "request")
	if e != nil || m.Content != "hello" || m.SenderUserID.String() != p.UserID || len(db.saved) != 1 || bus.calls != 1 {
		t.Fatal("persistence with Redis outage", m, e)
	}
	db.fail = true
	if _, e = s.Send(context.Background(), p, id, "failed", ""); e == nil || bus.calls != 1 {
		t.Fatal("published unsuccessful save")
	}
}
