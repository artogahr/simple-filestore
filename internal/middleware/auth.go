// Package middleware provides HTTP auth middleware using signed/encrypted cookies.
package middleware

import (
	"context"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
)

type contextKey int

const (
	folderKey contextKey = iota
)

// Auth manages user and admin sessions using gorilla/securecookie.
// Two cookies are used:
//   - sf_user:  signed+encrypted, contains the folder the user is browsing
//   - sf_admin: signed+encrypted, contains admin flag
type Auth struct {
	sc *securecookie.SecureCookie
}

const (
	userCookieName  = "sf_user"
	adminCookieName = "sf_admin"
	cookieMaxAge    = 86400 * 30 // 30 days
)

type userSession struct {
	Folder string `json:"f"`
}

type adminSession struct {
	Admin bool `json:"a"`
}

// New creates an Auth using the hex-encoded secretKey from config.
func New(secretKeyHex string) (*Auth, error) {
	key, err := hex.DecodeString(secretKeyHex)
	if err != nil || len(key) < 16 {
		// Fall back to using the raw string as key bytes if not valid hex
		key = []byte(secretKeyHex)
	}
	sc := securecookie.New(key, key)
	sc.MaxAge(cookieMaxAge)
	return &Auth{sc: sc}, nil
}

// SetUserSession encodes a signed cookie for the given folder.
func (a *Auth) SetUserSession(w http.ResponseWriter, folder string) error {
	encoded, err := a.sc.Encode(userCookieName, userSession{Folder: folder})
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     userCookieName,
		Value:    encoded,
		MaxAge:   cookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
	return nil
}

// GetUserSession decodes the user session cookie and returns the folder name.
func (a *Auth) GetUserSession(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(userCookieName)
	if err != nil {
		return "", false
	}
	var sess userSession
	if err := a.sc.Decode(userCookieName, cookie.Value, &sess); err != nil {
		return "", false
	}
	if sess.Folder == "" {
		return "", false
	}
	return sess.Folder, true
}

// ClearUserSession deletes the user session cookie.
func (a *Auth) ClearUserSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:    userCookieName,
		Value:   "",
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
		Path:    "/",
	})
}

// SetAdminSession encodes a signed cookie for admin access.
func (a *Auth) SetAdminSession(w http.ResponseWriter) error {
	encoded, err := a.sc.Encode(adminCookieName, adminSession{Admin: true})
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    encoded,
		MaxAge:   cookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
	return nil
}

// IsAdmin reports whether the request carries a valid admin session cookie.
func (a *Auth) IsAdmin(r *http.Request) bool {
	cookie, err := r.Cookie(adminCookieName)
	if err != nil {
		return false
	}
	var sess adminSession
	if err := a.sc.Decode(adminCookieName, cookie.Value, &sess); err != nil {
		return false
	}
	return sess.Admin
}

// ClearAdminSession deletes the admin session cookie.
func (a *Auth) ClearAdminSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:    adminCookieName,
		Value:   "",
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
		Path:    "/",
	})
}

// RequireUser is middleware that checks for a valid user session.
// The folder name is injected into the request context for downstream handlers.
func (a *Auth) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		folder, ok := a.GetUserSession(r)
		if !ok {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), folderKey, folder)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin is middleware that checks for a valid admin session.
func (a *Auth) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.IsAdmin(r) {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// FolderFromContext extracts the folder name injected by RequireUser.
func FolderFromContext(ctx context.Context) string {
	v, _ := ctx.Value(folderKey).(string)
	return v
}
