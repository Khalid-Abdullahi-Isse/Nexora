package postgres

import (
	"context"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/models"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ownership"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"log/slog"
	"time"
)

// Receipt and row commit atomically; concurrent replicas serialize on event_id.
func (d *Database) SaveEvent(ctx context.Context, id uuid.UUID, n *models.Item) (*models.Item, error) {
	var saved *models.Item
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r := tx.Exec("INSERT INTO notification_processed_events(event_id) VALUES(?) ON CONFLICT DO NOTHING", id)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			var rows []models.Item
			if err := tx.Where("event_id=?", id).Limit(1).Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) > 0 {
				rows[0].IsRead = rows[0].ReadAt != nil
				saved = &rows[0]
			}
			return nil
		}
		if n == nil {
			return nil
		}
		if err := tx.Create(n).Error; err != nil {
			return err
		}
		saved = n
		return nil
	})
	if err == nil && saved != nil {
		slog.Info("notification_persisted", "event_id", id, "notification_id", saved.ID, "recipient_id", saved.RecipientID)
	}
	return saved, err
}
func (d *Database) List(ctx context.Context, p authn.Principal, page, limit int) ([]models.Item, error) {
	rows := []models.Item{}
	if err := ownership.Authorize(p, "notifications.manage-own"); err != nil {
		return nil, err
	}
	err := d.db.WithContext(ctx).Where("user_id=?", p.UserID).Order("created_at DESC,id DESC").Offset((page - 1) * limit).Limit(limit + 1).Find(&rows).Error
	for i := range rows {
		rows[i].IsRead = rows[i].ReadAt != nil
	}
	return rows, err
}
func (d *Database) Unread(ctx context.Context, p authn.Principal) (int64, error) {
	if err := ownership.Authorize(p, "notifications.manage-own"); err != nil {
		return 0, err
	}
	var n int64
	err := d.db.WithContext(ctx).Model(&models.Item{}).Where("user_id=? AND read_at IS NULL", p.UserID).Count(&n).Error
	return n, err
}
func (d *Database) MarkAll(ctx context.Context, p authn.Principal) error {
	if err := ownership.Authorize(p, "notifications.manage-own"); err != nil {
		return err
	}
	return d.db.WithContext(ctx).Model(&models.Item{}).Where("user_id=? AND read_at IS NULL", p.UserID).Updates(map[string]any{"read_at": time.Now().UTC(), "updated_at": time.Now().UTC()}).Error
}
