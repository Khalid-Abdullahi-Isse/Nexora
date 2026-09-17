package postgres

import (
	"context"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/notificationevents"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ownership"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"sort"
	"strings"
	"time"
)

func (d *Database) Direct(ctx context.Context, p authn.Principal, participant string) (models.Conversation, error) {
	var out models.Conversation
	if e := ownership.Authorize(p, "chats.member", participant); e != nil {
		return out, e
	}
	participant = uuid.MustParse(participant).String()
	if participant == p.UserID {
		return out, ownership.ErrInvalid
	}
	pair := []string{p.UserID, participant}
	sort.Strings(pair)
	e := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if e := tx.Table("users").Where("id IN ? AND status = 'active'", pair).Count(&n).Error; e != nil {
			return ownership.ErrDatabase
		}
		if n != 2 {
			return ownership.ErrInvalid
		}
		// Unique pair index serializes simultaneous creates across replicas.
		r := tx.Raw(`INSERT INTO conversations(id,type,created_by_user_id,direct_pair) VALUES(?,'direct',?,?) ON CONFLICT(direct_pair) WHERE direct_pair IS NOT NULL DO UPDATE SET direct_pair=EXCLUDED.direct_pair RETURNING id,type,created_by_user_id,created_at,updated_at`, uuid.New(), p.UserID, strings.Join(pair, ":")).Scan(&out)
		if r.Error != nil {
			return ownership.ErrDatabase
		}
		for _, u := range pair {
			if e := tx.Exec(`INSERT INTO conversation_members(conversation_id,user_id) VALUES(?,?) ON CONFLICT(conversation_id,user_id) DO NOTHING`, out.ID, u).Error; e != nil {
				return ownership.ErrDatabase
			}
		}
		var active int64
		tx.Table("conversation_members").Where("conversation_id=? AND left_at IS NULL", out.ID).Count(&active)
		if active != 2 {
			return ownership.ErrForbidden
		}
		return nil
	})
	return out, e
}
func (d *Database) Conversations(ctx context.Context, p authn.Principal, limit int) ([]models.Conversation, error) {
	out := []models.Conversation{}
	if e := ownership.Authorize(p, "chats.member"); e != nil {
		return out, e
	}
	e := d.db.WithContext(ctx).Raw(`SELECT c.*, (SELECT json_build_object('id',m.id,'conversationId',m.conversation_id,'senderId',m.sender_user_id,'content',m.content,'messageType',m.message_type,'createdAt',m.created_at) FROM messages m WHERE m.conversation_id=c.id AND m.deleted_at IS NULL ORDER BY m.created_at DESC,m.id DESC LIMIT 1) AS last_message,
 (SELECT count(*) FROM messages m WHERE m.conversation_id=c.id AND m.deleted_at IS NULL AND m.sender_user_id<>? AND NOT EXISTS(SELECT 1 FROM message_receipts r WHERE r.message_id=m.id AND r.user_id=? AND r.read_at IS NOT NULL)) AS unread_count
 FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id WHERE cm.user_id=? AND cm.left_at IS NULL ORDER BY c.updated_at DESC,c.id DESC LIMIT ?`, p.UserID, p.UserID, p.UserID, limit).Scan(&out).Error
	return out, e
}
func (d *Database) Conversation(ctx context.Context, p authn.Principal, id string) (models.Conversation, error) {
	var out models.Conversation
	e := d.member(ctx, p, id, func(tx *gorm.DB, _ ConversationMember) error { return ownership.Result(tx.First(&out, "id=?", id)) })
	return out, e
}
func (d *Database) History(ctx context.Context, p authn.Principal, id, before string, limit int) ([]models.Message, error) {
	out := []models.Message{}
	e := d.member(ctx, p, id, func(tx *gorm.DB, _ ConversationMember) error {
		q := tx.Where("conversation_id=?", id)
		if before != "" {
			var cursor models.Message
			if e := ownership.Result(tx.Unscoped().First(&cursor, "id=? AND conversation_id=?", before, id)); e != nil {
				return ownership.ErrInvalid
			}
			q = q.Where("(created_at,id)<(?,?)", cursor.CreatedAt, cursor.ID)
		}
		return q.Order("created_at DESC,id DESC").Limit(limit).Find(&out).Error
	})
	return out, e
}
func (d *Database) Send(ctx context.Context, p authn.Principal, id, content string) (models.Message, error) {
	var out models.Message
	e := d.member(ctx, p, id, func(tx *gorm.DB, _ ConversationMember) error {
		now := time.Now().UTC()
		out = models.Message{ID: uuid.New(), ConversationID: uuid.MustParse(id), SenderUserID: uuid.MustParse(p.UserID), Content: content, MessageType: "text", CreatedAt: now, UpdatedAt: now}
		if e := tx.Create(&out).Error; e != nil {
			return ownership.ErrDatabase
		}
		if err := tx.Model(&Conversation{}).Where("id=?", id).Update("updated_at", now).Error; err != nil {
			return err
		}
		var recipients []uuid.UUID
		if err := tx.Model(&ConversationMember{}).Where("conversation_id=? AND left_at IS NULL AND user_id<>?", id, p.UserID).Pluck("user_id", &recipients).Error; err != nil {
			return err
		}
		for _, recipient := range recipients {
			event := notificationevents.Event{EventID: uuid.New(), EventType: "chat.message.created", RecipientID: recipient, ActorID: &out.SenderUserID, EntityID: &out.ConversationID, EntityType: "conversation", CreatedAt: now, Metadata: map[string]string{"messageId": out.ID.String()}}
			if err := notificationevents.Enqueue(tx, "chat", event); err != nil {
				return err
			}
		}
		return nil
	})
	return out, e
}
func (d *Database) MessageConversation(ctx context.Context, p authn.Principal, id string) (string, error) {
	if e := ownership.Authorize(p, "chats.member", id); e != nil {
		return "", e
	}
	var m models.Message
	e := ownership.Result(d.db.WithContext(ctx).Where("id=? AND EXISTS(SELECT 1 FROM conversation_members cm WHERE cm.conversation_id=messages.conversation_id AND cm.user_id=? AND cm.left_at IS NULL)", id, p.UserID).First(&m))
	return m.ConversationID.String(), e
}
func (d *Database) Receipt(ctx context.Context, p authn.Principal, conversation, id string, read bool) (models.MessageReceipt, error) {
	var out models.MessageReceipt
	e := d.member(ctx, p, conversation, func(tx *gorm.DB, _ ConversationMember) error {
		var m models.Message
		if e := ownership.Result(tx.First(&m, "id=? AND conversation_id=?", id, conversation)); e != nil {
			return e
		}
		var readAt *time.Time
		now := time.Now().UTC()
		if read {
			readAt = &now
		}
		return tx.Raw(`INSERT INTO message_receipts(message_id,user_id,delivered_at,read_at) VALUES(?,?,?,?) ON CONFLICT(message_id,user_id) DO UPDATE SET delivered_at=COALESCE(message_receipts.delivered_at,EXCLUDED.delivered_at),read_at=COALESCE(message_receipts.read_at,EXCLUDED.read_at) RETURNING *`, id, p.UserID, now, readAt).Scan(&out).Error
	})
	return out, e
}
func (d *Database) Delete(ctx context.Context, p authn.Principal, conversation, id string) error {
	return d.member(ctx, p, conversation, func(tx *gorm.DB, _ ConversationMember) error {
		return ownership.Result(tx.Where("id=? AND conversation_id=? AND sender_user_id=?", id, conversation, p.UserID).Delete(&models.Message{}))
	})
}
func (d *Database) Recipients(ctx context.Context, p authn.Principal, id string) ([]string, error) {
	out := []string{}
	e := d.member(ctx, p, id, func(tx *gorm.DB, _ ConversationMember) error {
		return tx.Model(&ConversationMember{}).Where("conversation_id=? AND left_at IS NULL", id).Pluck("user_id", &out).Error
	})
	return out, e
}
