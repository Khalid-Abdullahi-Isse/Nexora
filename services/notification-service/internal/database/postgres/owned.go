package postgres

import (
	"context"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ownership"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
	"time"
)

func (d *Database) GetOwnedNotification(ctx context.Context, p authn.Principal, id string) (Notification, error) {
	var row Notification
	if e := ownership.Authorize(p, "notifications.manage-own", id); e != nil {
		return row, e
	}
	e := ownership.Result(d.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, p.UserID).First(&row))
	return row, e
}
func (d *Database) MarkOwnedRead(ctx context.Context, p authn.Principal, id string) error {
	if e := ownership.Authorize(p, "notifications.manage-own", id); e != nil {
		return e
	}
	return ownership.Result(d.db.WithContext(ctx).Model(&Notification{}).Where("id = ? AND user_id = ?", id, p.UserID).Updates(map[string]any{"read_at": time.Now().UTC(), "updated_at": time.Now().UTC()}))
}
func (d *Database) DeleteOwnedNotification(ctx context.Context, p authn.Principal, id string) error {
	if e := ownership.Authorize(p, "notifications.manage-own", id); e != nil {
		return e
	}
	return ownership.Result(d.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, p.UserID).Delete(&Notification{}))
}
func (d *Database) ListOwnedDeliveries(ctx context.Context, p authn.Principal, id string) ([]NotificationDelivery, error) {
	rows := []NotificationDelivery{}
	if _, e := d.GetOwnedNotification(ctx, p, id); e != nil {
		return rows, e
	}
	r := d.db.WithContext(ctx).Where("notification_id = ? AND notification_id IN (SELECT id FROM notifications WHERE user_id = ?)", id, p.UserID).Limit(100).Find(&rows)
	if r.Error != nil {
		return nil, ownership.ErrDatabase
	}
	return rows, nil
}
func (d *Database) SetOwnPreference(ctx context.Context, p authn.Principal, kind NotificationType, enabled bool) error {
	if e := ownership.Authorize(p, "notifications.manage-own"); e != nil {
		return e
	}
	if kind != NotificationTypeFollow && kind != NotificationTypeLike && kind != NotificationTypeComment && kind != NotificationTypeMessage {
		return ownership.ErrInvalid
	}
	row := NotificationPreference{UserID: uuid.MustParse(p.UserID), NotificationType: kind, InAppEnabled: enabled, Timestamps: Timestamps{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}}
	r := d.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "notification_type"}}, DoUpdates: clause.AssignmentColumns([]string{"in_app_enabled", "updated_at"})}).Create(&row)
	return ownership.Result(r)
}
