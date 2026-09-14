package http

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/httpsecurity"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ratelimit"
	"github.com/gin-gonic/gin"
)

type Security struct {
	Roles        *service.RoleService
	Auth         *service.AuthService
	Verifier     *authn.Verifier
	Origins      httpsecurity.Origins
	SecureCookie bool
	Limiter      *ratelimit.Limiter
	RateConfig   ratelimit.Config
}

func (c *Controller) ConfigureSecurity(s Security) { c.security = &s }
func (c *Controller) cookieName() string {
	if c.security.SecureCookie {
		return "__Secure-refresh"
	}
	return "dev-refresh"
}
func (c *Controller) setCookie(ctx *gin.Context, raw string, expiry time.Time) {
	maxAge := -1
	if raw != "" {
		maxAge = int(time.Until(expiry).Seconds())
		if maxAge < 1 {
			maxAge = 1
		}
	}
	http.SetCookie(ctx.Writer, &http.Cookie{Name: c.cookieName(), Value: raw, Path: "/api/v1/auth", Expires: expiry, MaxAge: maxAge, HttpOnly: true, Secure: c.security.SecureCookie, SameSite: http.SameSiteLaxMode})
}
func meta(ctx *gin.Context) service.AuditMeta {
	return service.AuditMeta{IP: ctx.ClientIP(), UserAgent: ctx.Request.UserAgent()}
}
func authFailure(ctx *gin.Context, e error) {
	switch {
	case errors.Is(e, service.ErrCredentials):
		authn.Deny(ctx, 401, "INVALID_CREDENTIALS", "Invalid email or password")
	case errors.Is(e, service.ErrSession):
		authn.Deny(ctx, 401, "UNAUTHORIZED", "Authentication required")
	case errors.Is(e, service.ErrForbidden):
		authn.Deny(ctx, 403, "FORBIDDEN", "Permission denied")
	case errors.Is(e, service.ErrNotFound):
		authn.Deny(ctx, 404, "NOT_FOUND", "Resource not found")
	case errors.Is(e, service.ErrInvalidInput):
		authn.Deny(ctx, 400, "VALIDATION_ERROR", "Invalid input")
	default:
		authn.Deny(ctx, 500, "INTERNAL_ERROR", "Unable to process request")
	}
}
func (c *Controller) browser(ctx *gin.Context) bool {
	if !c.security.Origins.BrowserMutation(ctx) {
		authn.Deny(ctx, 403, "FORBIDDEN", "Origin and CSRF protection header required")
		return false
	}
	return true
}
func (c *Controller) refreshCookie(ctx *gin.Context) (string, bool) {
	if !c.browser(ctx) {
		return "", false
	}
	var raw string
	count := 0
	for _, cookie := range ctx.Request.Cookies() {
		if cookie.Name == c.cookieName() {
			count++
			raw = cookie.Value
		}
	}
	if count != 1 || !service.ValidRefresh(raw) {
		authFailure(ctx, service.ErrSession)
		return "", false
	}
	if !service.CheckCSRF(raw, ctx.GetHeader("X-CSRF-Token")) {
		authn.Deny(ctx, 403, "FORBIDDEN", "Invalid CSRF token")
		return "", false
	}
	return raw, true
}
func (c *Controller) extraLimit(ctx *gin.Context, p ratelimit.Policy, key string) bool {
	if c.security.Limiter == nil || !c.security.RateConfig.Enabled {
		return true
	}

	result, e := c.security.Limiter.Check(ctx.Request.Context(), p, key)
	if e != nil {
		ctx.Header("Retry-After", "1")
		authn.Deny(ctx, 503, "UNAVAILABLE", "Please try again later")
		return false
	}
	if !result.Allowed {
		ctx.Header("Retry-After", strconv.FormatInt(int64((result.Reset+time.Second-1)/time.Second), 10))
		authn.Deny(ctx, 429, "RATE_LIMITED", "Rate limit exceeded")
		return false
	}
	return true
}
func (c *Controller) Login(ctx *gin.Context) {
	if !c.browser(ctx) {
		return
	}
	var req RegisterRequest
	if httpsecurity.Decode(ctx, &req) != nil {
		authFailure(ctx, service.ErrInvalidInput)
		return
	}
	key := "account:" + strings.ToLower(strings.TrimSpace(req.Email))
	if !c.extraLimit(ctx, c.security.RateConfig.LoginAccount, key) {
		return
	}
	tokens, e := c.security.Auth.Login(ctx.Request.Context(), req.Email, req.Password, meta(ctx))
	if e != nil {
		authFailure(ctx, e)
		return
	}
	c.setCookie(ctx, tokens.RefreshToken, tokens.RefreshExpiresAt)
	ctx.JSON(200, SuccessResponse{Success: true, Data: tokens})
}
func (c *Controller) Refresh(ctx *gin.Context) {
	raw, ok := c.refreshCookie(ctx)
	if !ok {
		return
	}
	// Per-credential limit complements IP throttling; family reuse is serialized in DB.
	h := sha256.Sum256([]byte(raw))
	if !c.extraLimit(ctx, c.security.RateConfig.RefreshAccount, "refresh:"+hex.EncodeToString(h[:])) {
		return
	}
	tokens, e := c.security.Auth.Refresh(ctx.Request.Context(), raw, meta(ctx))
	if e != nil {
		if errors.Is(e, service.ErrSession) {
			c.setCookie(ctx, "", time.Unix(1, 0))
		}
		authFailure(ctx, e)
		return
	}
	c.setCookie(ctx, tokens.RefreshToken, tokens.RefreshExpiresAt)
	ctx.JSON(200, SuccessResponse{Success: true, Data: tokens})
}
func (c *Controller) Logout(ctx *gin.Context) {
	raw, ok := c.refreshCookie(ctx)
	if !ok {
		return
	}
	if e := c.security.Auth.Logout(ctx.Request.Context(), raw, meta(ctx)); e != nil {
		authFailure(ctx, e)
		return
	}
	c.setCookie(ctx, "", time.Unix(1, 0))
	ctx.Status(204)
}
func (c *Controller) LogoutAll(ctx *gin.Context) {
	p, _ := authn.FromContext(ctx)
	if e := c.security.Auth.LogoutAll(ctx.Request.Context(), p, meta(ctx)); e != nil {
		authFailure(ctx, e)
		return
	}
	c.setCookie(ctx, "", time.Unix(1, 0))
	ctx.Status(204)
}
func (c *Controller) Sessions(ctx *gin.Context) {
	p, _ := authn.FromContext(ctx)
	rows, e := c.security.Auth.Sessions(ctx.Request.Context(), p)
	if e != nil {
		authFailure(ctx, e)
		return
	}
	ctx.JSON(200, SuccessResponse{Success: true, Data: rows})
}
func (c *Controller) RevokeSession(ctx *gin.Context) {
	p, _ := authn.FromContext(ctx)
	if e := c.security.Auth.RevokeSession(ctx.Request.Context(), p, ctx.Param("id"), meta(ctx)); e != nil {
		authFailure(ctx, e)
		return
	}
	ctx.Status(204)
}
func (c *Controller) ChangePassword(ctx *gin.Context) {
	p, _ := authn.FromContext(ctx)
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if httpsecurity.Decode(ctx, &req) != nil {
		authFailure(ctx, service.ErrInvalidInput)
		return
	}
	if !c.extraLimit(ctx, c.security.RateConfig.LoginAccount, "password:"+p.UserID) {
		return
	}
	if e := c.security.Auth.ChangePassword(ctx.Request.Context(), p, req.CurrentPassword, req.NewPassword, meta(ctx)); e != nil {
		authFailure(ctx, e)
		return
	}
	c.setCookie(ctx, "", time.Unix(1, 0))
	ctx.Status(204)
}
func (c *Controller) Me(ctx *gin.Context) {
	p, _ := authn.FromContext(ctx)
	u, e := c.users.FindUserByID(ctx.Request.Context(), p.UserID)
	if e != nil {
		authFailure(ctx, e)
		return
	}
	ctx.JSON(200, SuccessResponse{Success: true, Data: UserResponse{ID: u.ID, Email: u.Email, Status: string(u.Status), CreatedAt: u.CreatedAt}})
}

func (c *Controller) AssignRole(ctx *gin.Context) {
	if e := c.security.Roles.AssignRoleToUser(ctx.Request.Context(), ctx.Param("id"), ctx.Param("roleID")); e != nil {
		authFailure(ctx, e)
		return
	}
	ctx.Status(204)
}
func (c *Controller) RemoveRole(ctx *gin.Context) {
	if e := c.security.Roles.RemoveRoleFromUser(ctx.Request.Context(), ctx.Param("id"), ctx.Param("roleID")); e != nil {
		authFailure(ctx, e)
		return
	}
	ctx.Status(204)
}

// CSRF restores the session-bound CSRF value after a browser reload. The trusted
// Origin and required custom header protect this bootstrap; it issues no access or refresh credentials.
func (c *Controller) CSRF(ctx *gin.Context) {
	if !c.browser(ctx) {
		return
	}
	raw := ""
	count := 0
	for _, cookie := range ctx.Request.Cookies() {
		if cookie.Name == c.cookieName() {
			raw = cookie.Value
			count++
		}
	}
	if count != 1 || !service.ValidRefresh(raw) {
		authFailure(ctx, service.ErrSession)
		return
	}
	ctx.JSON(200, SuccessResponse{Success: true, Data: gin.H{"csrf_token": service.CSRFToken(raw)}})
}
