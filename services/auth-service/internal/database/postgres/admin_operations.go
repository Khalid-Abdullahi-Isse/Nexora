package postgres

import (
	"context"
	"sort"
	"time"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"gorm.io/gorm"
)

// Admin checks are repeated against current database state under user locks.
// Sorted locks avoid actor/target deadlocks; refresh and logout lock the same users.
func (d *Database) adminMutation(ctx context.Context, target, action, resource string, change func(*gorm.DB) error) error {
	actor, ok := authn.Actor(ctx)
	age := time.Since(time.Unix(actor.AuthTime, 0))
	if !ok || !actor.HasRole("admin") || !actor.Can("admin.users.manage") || age < 0 || age > 5*time.Minute {
		return service.ErrForbidden
	}
	return authError(d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids := []string{actor.UserID}
		if target != "" && target != actor.UserID {
			ids = append(ids, target)
		}
		sort.Strings(ids)
		var acting User
		for _, id := range ids {
			u, e := lockUser(tx, id)
			if e != nil {
				return e
			}
			if id == actor.UserID {
				acting = u
			}
		}
		p, e := principal(tx, acting, SessionFamily{ID: actor.SessionID, CreatedAt: time.Unix(actor.AuthTime, 0)})
		if e != nil {
			return e
		}
		if acting.Status != UserStatusActive || !p.HasRole("admin") || !p.Can("admin.users.manage") {
			return service.ErrForbidden
		}
		if e = change(tx); e != nil {
			return e
		}
		return audit(tx, actor.UserID, target, action, resource, service.AuditMeta{})
	}))
}
