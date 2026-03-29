// Package handlers contains all HTTP handlers for simple-filestore.
package handlers

import (
	"html/template"
	"net/http"

	"github.com/artogahr/simple-filestore/internal/config"
	"github.com/artogahr/simple-filestore/internal/middleware"
	"github.com/artogahr/simple-filestore/internal/storage"
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	cfg     *config.Config
	cfgPath string // path to config.json, for saving changes
	store   *storage.Store
	auth    *middleware.Auth
	tmpl    *template.Template
	deleted string // path to workspace/deleted/
}

// New creates a Handler.
func New(
	cfg *config.Config,
	cfgPath string,
	store *storage.Store,
	auth *middleware.Auth,
	tmpl *template.Template,
	deletedDir string,
) *Handler {
	return &Handler{
		cfg:     cfg,
		cfgPath: cfgPath,
		store:   store,
		auth:    auth,
		tmpl:    tmpl,
		deleted: deletedDir,
	}
}

// RegisterRoutes registers all routes on mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Public routes
	mux.HandleFunc("GET /{$}", h.loginPage)
	mux.HandleFunc("POST /login", h.postLogin)
	mux.HandleFunc("GET /logout", h.logout)
	mux.HandleFunc("GET /admin/login", h.adminLoginPage)
	mux.HandleFunc("POST /admin/login", h.postAdminLogin)
	mux.HandleFunc("GET /admin/logout", h.adminLogout)

	// User-authenticated routes
	mux.Handle("GET /browse/", h.auth.RequireUser(http.HandlerFunc(h.browse)))
	mux.Handle("GET /browse", h.auth.RequireUser(http.HandlerFunc(h.browseRoot)))
	mux.Handle("GET /files/", h.auth.RequireUser(http.HandlerFunc(h.download)))
	mux.Handle("POST /upload", h.auth.RequireUser(http.HandlerFunc(h.upload)))
	mux.Handle("POST /mkdir", h.auth.RequireUser(http.HandlerFunc(h.mkdir)))
	mux.Handle("POST /rename", h.auth.RequireUser(http.HandlerFunc(h.rename)))
	mux.Handle("DELETE /files/", h.auth.RequireUser(http.HandlerFunc(h.deleteFile)))
	mux.Handle("GET /preview/", h.auth.RequireUser(http.HandlerFunc(h.preview)))
	mux.Handle("GET /trash", h.auth.RequireUser(http.HandlerFunc(h.trashList)))
	mux.Handle("POST /restore", h.auth.RequireUser(http.HandlerFunc(h.restore)))
	mux.Handle("DELETE /trash-item/", h.auth.RequireUser(http.HandlerFunc(h.permanentDelete)))

	// Admin routes
	mux.Handle("GET /admin", h.auth.RequireAdmin(http.HandlerFunc(h.adminPanel)))
	mux.Handle("POST /admin/folders", h.auth.RequireAdmin(http.HandlerFunc(h.adminCreateFolder)))
	mux.Handle("DELETE /admin/folders/", h.auth.RequireAdmin(http.HandlerFunc(h.adminDeleteFolder)))
}

// renderError writes an error response. For HTMX requests it sends a minimal
// fragment; otherwise a full error page.
func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(status)
		template.HTMLEscape(w, []byte(`<div class="error-toast">`+msg+`</div>`))
		return
	}
	w.WriteHeader(status)
	_ = h.tmpl.ExecuteTemplate(w, "error.html", map[string]any{
		"Status":  status,
		"Message": msg,
	})
}

// isHTMX reports whether the request was made by HTMX.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
