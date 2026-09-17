package service

import (
	events "github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/notificationevents"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestEventRules(t *testing.T) {
	actor, entity := uuid.New(), uuid.New()
	for typ, r := range rules {
		e := events.Event{EventID: uuid.New(), EventType: typ, RecipientID: uuid.New(), ActorID: &actor, EntityID: &entity, EntityType: r.entity, CreatedAt: time.Now(), Metadata: map[string]string{"actorName": "Ahmed"}}
		n, err := Build(e)
		if err != nil || n == nil || n.Type != r.kind || n.IsRead {
			t.Fatalf("%s: %v %v", typ, n, err)
		}
		if r.actor {
			e.RecipientID = actor
			n, err = Build(e)
			if err != nil || n != nil {
				t.Fatal("self notification", err)
			}
		}
	}
	e := events.Event{EventID: uuid.New(), EventType: "unknown", RecipientID: uuid.New(), CreatedAt: time.Now()}
	if _, err := Build(e); err == nil {
		t.Fatal("unknown event accepted")
	}
	e.EventType = "post.liked"
	if _, err := Build(e); err == nil {
		t.Fatal("missing actor accepted")
	}
}
