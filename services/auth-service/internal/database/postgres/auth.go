package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SessionFamily struct {
	ID        string `gorm:"type:uuid;primaryKey"`
	UserID    string `gorm:"type:uuid"`
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
}

func (SessionFamily) TableName() string { return "session_families" }

type SecurityAudit struct {
	ID           string  `gorm:"type:uuid;primaryKey"`
	ActorUserID  *string `gorm:"type:uuid"`
	TargetUserID *string `gorm:"type:uuid"`
	Action       string
	ResourceID   *string `gorm:"type:uuid"`
	CreatedAt    time.Time
	IP           string
	UserAgent    string
}

func (SecurityAudit) TableName() string { return "security_audit" }
func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func bounded(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
func audit(tx *gorm.DB, actor, target, action, resource string, meta service.AuditMeta) error {
	return tx.Create(&SecurityAudit{ID: uuid.NewString(), ActorUserID: ptr(actor), TargetUserID: ptr(target), Action: action, ResourceID: ptr(resource), CreatedAt: time.Now().UTC(), IP: bounded(meta.IP, 64), UserAgent: bounded(meta.UserAgent, 256)}).Error
}
func lockUser(tx *gorm.DB, id string) (User, error) {
	var u User
	e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&u).Error
	return u, e
}
func principal(tx *gorm.DB, u User, f SessionFamily) (authn.Principal, error) {
	p := authn.Principal{UserID: u.ID.String(), SessionID: f.ID, AuthTime: f.CreatedAt.Unix(), Roles: []string{}, Permissions: []string{}}
	e := tx.Table("roles").Joins("JOIN user_roles ON user_roles.role_id=roles.id").Where("user_roles.user_id = ?", u.ID).Order("roles.name").Pluck("roles.name", &p.Roles).Error
	if e != nil {
		return p, e
	}
	e = tx.Table("permissions").Distinct("permissions.code").Joins("JOIN role_permissions ON role_permissions.permission_id=permissions.id").Joins("JOIN user_roles ON user_roles.role_id=role_permissions.role_id").Where("user_roles.user_id = ?", u.ID).Order("permissions.code").Pluck("permissions.code", &p.Permissions).Error
	return p, e
}
func (d *Database) StartSession(ctx context.Context, r service.SessionRequest, mint service.Mint) (service.SessionResult, error) {
	var out service.SessionResult
	e := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		u, e := lockUser(tx, r.UserID)
		if e != nil {
			return e
		}
		if u.Status != UserStatusActive || u.PasswordHash != r.PasswordHash {
			return service.ErrCredentials
		}
		now := time.Now().UTC().Truncate(time.Second)
		f := SessionFamily{ID: r.FamilyID, UserID: r.UserID, CreatedAt: now, ExpiresAt: now.Add(r.TTL)}
		if e = tx.Create(&f).Error; e != nil {
			return e
		}
		p, e := principal(tx, u, f)
		if e != nil {
			return e
		}
		out.AccessToken, e = mint(p, now)
		if e != nil {
			return e
		}
		token := Session{ID: uuid.MustParse(r.TokenID), UserID: u.ID, TokenHash: r.NewHash, FamilyID: &f.ID, ExpiresAt: f.ExpiresAt, Timestamps: Timestamps{CreatedAt: now, UpdatedAt: now}}
		if e = tx.Create(&token).Error; e != nil {
			return e
		}
		if e = tx.Model(&u).Update("last_login_at", now).Error; e != nil {
			return e
		}
		out.ExpiresAt = f.ExpiresAt
		return audit(tx, r.UserID, r.UserID, "LOGIN_SUCCESS", f.ID, r.Meta)
	})
	return out, authError(e)
}
func authError(e error) error {
	if e == nil {
		return nil
	}
	for _, known := range []error{service.ErrSession, service.ErrCredentials, service.ErrForbidden, service.ErrNotFound} {
		if errors.Is(e, known) {
			return known
		}
	}
	return databaseError(e)
}
func (d *Database) RotateSession(ctx context.Context, r service.SessionRequest, mint service.Mint) (service.SessionResult, error) {
	var out service.SessionResult
	reused := false
	e := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Initial lookup only discovers immutable lock IDs. All state is re-read under locks.
		var token Session
		if e := tx.Where("token_hash = ?", r.OldHash).First(&token).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return service.ErrSession
			}
			return e
		}
		if token.FamilyID == nil {
			return service.ErrSession
		}
		u, e := lockUser(tx, token.UserID.String())
		if e != nil {
			return e
		}
		var f SessionFamily
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", *token.FamilyID, u.ID).First(&f).Error; e != nil {
			return e
		}
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", token.ID).First(&token).Error; e != nil {
			return e
		}
		now := time.Now().UTC()
		if token.ConsumedAt != nil {
			reused = true
			if e = revokeFamily(tx, f.ID, now); e != nil {
				return e
			}
			return audit(tx, u.ID.String(), u.ID.String(), "REFRESH_TOKEN_REUSE_DETECTED", f.ID, r.Meta)
		}
		if u.Status != UserStatusActive || f.RevokedAt != nil || token.RevokedAt != nil || !now.Before(f.ExpiresAt) || !now.Before(token.ExpiresAt) {
			return service.ErrSession
		}
		if e = tx.Model(&token).Updates(map[string]any{"consumed_at": now, "updated_at": now}).Error; e != nil {
			return e
		}
		next := Session{ID: uuid.MustParse(r.TokenID), UserID: u.ID, TokenHash: r.NewHash, FamilyID: &f.ID, ExpiresAt: f.ExpiresAt, Timestamps: Timestamps{CreatedAt: now, UpdatedAt: now}}
		if e = tx.Create(&next).Error; e != nil {
			return e
		}
		if e = tx.Model(&token).Update("replaced_by_token_id", next.ID).Error; e != nil {
			return e
		}
		p, e := principal(tx, u, f)
		if e != nil {
			return e
		}
		out.AccessToken, e = mint(p, now)
		if e != nil {
			return e
		}
		out.ExpiresAt = f.ExpiresAt
		return audit(tx, u.ID.String(), u.ID.String(), "TOKEN_REFRESHED", f.ID, r.Meta)
	})
	if e != nil {
		return out, authError(e)
	}
	if reused {
		return service.SessionResult{}, service.ErrSession
	}
	return out, nil
}
func revokeFamily(tx *gorm.DB, id string, now time.Time) error {
	if e := tx.Model(&SessionFamily{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", now).Error; e != nil {
		return e
	}
	return tx.Model(&Session{}).Where("family_id = ? AND revoked_at IS NULL", id).Update("revoked_at", now).Error
}
func revokeAll(tx *gorm.DB, user string, now time.Time) error {
	if e := tx.Model(&SessionFamily{}).Where("user_id = ? AND revoked_at IS NULL", user).Update("revoked_at", now).Error; e != nil {
		return e
	}
	return tx.Model(&Session{}).Where("user_id = ? AND revoked_at IS NULL", user).Update("revoked_at", now).Error
}
func (d *Database) Logout(ctx context.Context, hash string, meta service.AuditMeta) error {
	return authError(d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var token Session
		e := tx.Where("token_hash = ?", hash).First(&token).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil
		}
		if e != nil {
			return e
		}
		if token.FamilyID == nil {
			return nil
		}
		if _, e = lockUser(tx, token.UserID.String()); e != nil {
			return e
		}
		if e = revokeFamily(tx, *token.FamilyID, time.Now().UTC()); e != nil {
			return e
		}
		return audit(tx, token.UserID.String(), token.UserID.String(), "LOGOUT", *token.FamilyID, meta)
	}))
}
func (d *Database) LogoutAll(ctx context.Context, user string, meta service.AuditMeta) error {
	return authError(d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, e := lockUser(tx, user); e != nil {
			return e
		}
		if e := revokeAll(tx, user, time.Now().UTC()); e != nil {
			return e
		}
		return audit(tx, user, user, "LOGOUT_ALL", "", meta)
	}))
}
func (d *Database) ListSessions(ctx context.Context, user string) ([]service.SessionInfo, error) {
	rows := []service.SessionInfo{}
	e := d.db.WithContext(ctx).Model(&SessionFamily{}).Select("id,created_at,expires_at,revoked_at").Where("user_id = ? AND expires_at > ? AND revoked_at IS NULL", user, time.Now().UTC()).Order("created_at DESC").Limit(100).Scan(&rows).Error
	return rows, databaseError(e)
}
func (d *Database) RevokeOwnedSession(ctx context.Context, user, id string, meta service.AuditMeta) error {
	return authError(d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, e := lockUser(tx, user); e != nil {
			return e
		}
		var f SessionFamily
		if e := tx.Where("id = ? AND user_id = ?", id, user).First(&f).Error; e != nil {
			return e
		}
		if e := revokeFamily(tx, f.ID, time.Now().UTC()); e != nil {
			return e
		}
		return audit(tx, user, user, "SESSION_REVOKED", id, meta)
	}))
}
func (d *Database) ChangePassword(ctx context.Context, user, oldHash, newHash string, meta service.AuditMeta) error {
	return authError(d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		u, e := lockUser(tx, user)
		if e != nil {
			return e
		}
		if u.Status != UserStatusActive || u.PasswordHash != oldHash {
			return service.ErrCredentials
		}
		if e = tx.Model(&u).Updates(map[string]any{"password_hash": newHash, "updated_at": time.Now().UTC()}).Error; e != nil {
			return e
		}
		if e = revokeAll(tx, user, time.Now().UTC()); e != nil {
			return e
		}
		return audit(tx, user, user, "PASSWORD_CHANGED", user, meta)
	}))
}
func (d *Database) AuditFailure(ctx context.Context, action string, meta service.AuditMeta) error {
	return databaseError(audit(d.db.WithContext(ctx), "", "", action, "", meta))
}
