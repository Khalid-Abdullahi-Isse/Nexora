package postgres

import (
	"context"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ownership"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

// Serialize membership-sensitive operations on the conversation row, then check
// active membership. All membership mutations must acquire this same lock.
func (d *Database) member(ctx context.Context, p authn.Principal, id string, fn func(*gorm.DB, ConversationMember) error) error {
	if e := ownership.Authorize(p, "chats.member", id); e != nil {
		return e
	}
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c Conversation
		if e := ownership.Result(tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND EXISTS (SELECT 1 FROM conversation_members WHERE conversation_id = conversations.id AND user_id = ? AND left_at IS NULL)", id, p.UserID).First(&c)); e != nil {
			return e
		}
		var m ConversationMember
		if e := ownership.Result(tx.Where("conversation_id = ? AND user_id = ? AND left_at IS NULL", id, p.UserID).First(&m)); e != nil {
			return e
		}
		return fn(tx, m)
	})
}
func (d *Database) GetMemberConversation(ctx context.Context, p authn.Principal, id string) (Conversation, error) {
	var c Conversation
	e := d.member(ctx, p, id, func(tx *gorm.DB, _ ConversationMember) error {
		return ownership.Result(tx.Where("id = ?", id).First(&c))
	})
	return c, e
}
func (d *Database) ListMemberMessages(ctx context.Context, p authn.Principal, id string, limit int) ([]Message, error) {
	rows := []Message{}
	if limit < 1 || limit > 100 {
		return rows, ownership.ErrInvalid
	}
	e := d.member(ctx, p, id, func(tx *gorm.DB, _ ConversationMember) error {
		if tx.Where("conversation_id = ?", id).Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error != nil {
			return ownership.ErrDatabase
		}
		return nil
	})
	return rows, e
}
func (d *Database) CreateMemberMessage(ctx context.Context, p authn.Principal, id, content string) (Message, error) {
	var row Message
	if strings.TrimSpace(content) == "" || len(content) > 10000 {
		return row, ownership.ErrInvalid
	}
	e := d.member(ctx, p, id, func(tx *gorm.DB, _ ConversationMember) error {
		row = Message{ID: uuid.New(), ConversationID: uuid.MustParse(id), SenderUserID: uuid.MustParse(p.UserID), Content: content, Timestamps: Timestamps{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}}
		return ownership.Result(tx.Create(&row))
	})
	return row, e
}
func (d *Database) UpdateOwnMessage(ctx context.Context, p authn.Principal, conversation, id, content string) error {
	if e := ownership.Authorize(p, "chats.member", id); e != nil {
		return e
	}
	if strings.TrimSpace(content) == "" || len(content) > 10000 {
		return ownership.ErrInvalid
	}
	return d.member(ctx, p, conversation, func(tx *gorm.DB, _ ConversationMember) error {
		return ownership.Result(tx.Model(&Message{}).Where("id = ? AND conversation_id = ? AND sender_user_id = ?", id, conversation, p.UserID).Updates(map[string]any{"content": content, "updated_at": time.Now().UTC()}))
	})
}
func (d *Database) DeleteOwnMessage(ctx context.Context, p authn.Principal, conversation, id string) error {
	if e := ownership.Authorize(p, "chats.member", id); e != nil {
		return e
	}
	return d.member(ctx, p, conversation, func(tx *gorm.DB, _ ConversationMember) error {
		return ownership.Result(tx.Where("id = ? AND conversation_id = ? AND sender_user_id = ?", id, conversation, p.UserID).Delete(&Message{}))
	})
}
func (d *Database) RemoveMember(ctx context.Context, p authn.Principal, conversation, userID string) error {
	if e := ownership.Authorize(p, "chats.member", userID); e != nil {
		return e
	}
	return d.member(ctx, p, conversation, func(tx *gorm.DB, m ConversationMember) error {
		if p.UserID != userID && m.Role != ConversationMemberRoleAdmin {
			return ownership.ErrForbidden
		}
		return ownership.Result(tx.Model(&ConversationMember{}).Where("conversation_id = ? AND user_id = ? AND left_at IS NULL", conversation, userID).Update("left_at", time.Now().UTC()))
	})
}
