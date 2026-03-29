package handlers

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/artogahr/simple-filestore/internal/config"
)

// folderNameRe validates folder names: alphanumeric, hyphen, underscore, 1-64 chars.
var folderNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func (h *Handler) adminPanel(w http.ResponseWriter, r *http.Request) {
	_ = h.tmpl.ExecuteTemplate(w, "admin.html", map[string]any{
		"Folders": h.cfg.Folders,
	})
}

func (h *Handler) adminCreateFolder(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if !folderNameRe.MatchString(name) {
		h.adminError(w, r, "Invalid folder name. Use letters, numbers, hyphens, underscores (max 64 chars).")
		return
	}
	if h.cfg.HasFolder(name) {
		h.adminError(w, r, fmt.Sprintf("Folder %q already exists.", name))
		return
	}
	if err := h.store.CreateFolder(name); err != nil {
		h.adminError(w, r, fmt.Sprintf("Could not create folder on disk: %v", err))
		return
	}
	h.cfg.AddFolder(name)
	if err := config.Save(h.cfgPath, h.cfg); err != nil {
		// Folder was created on disk but config save failed — log but continue
		fmt.Printf("warning: failed to save config after creating folder %q: %v\n", name, err)
	}

	if isHTMX(r) {
		_ = h.tmpl.ExecuteTemplate(w, "admin-folder-list", map[string]any{
			"Folders": h.cfg.Folders,
		})
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *Handler) adminDeleteFolder(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/admin/folders/")
	name = strings.TrimSpace(name)

	if !h.cfg.HasFolder(name) {
		h.adminError(w, r, fmt.Sprintf("Folder %q not found.", name))
		return
	}
	if err := h.store.DeleteFolder(h.deleted, name); err != nil {
		h.adminError(w, r, fmt.Sprintf("Could not move folder to deleted: %v", err))
		return
	}
	h.cfg.RemoveFolder(name)
	if err := config.Save(h.cfgPath, h.cfg); err != nil {
		fmt.Printf("warning: failed to save config after deleting folder %q: %v\n", name, err)
	}

	if isHTMX(r) {
		_ = h.tmpl.ExecuteTemplate(w, "admin-folder-list", map[string]any{
			"Folders": h.cfg.Folders,
		})
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *Handler) adminError(w http.ResponseWriter, r *http.Request, msg string) {
	if isHTMX(r) {
		w.Header().Set("HX-Reswap", "none")
		w.WriteHeader(http.StatusBadRequest)
		_ = h.tmpl.ExecuteTemplate(w, "admin-error", map[string]any{"Error": msg})
		return
	}
	_ = h.tmpl.ExecuteTemplate(w, "admin.html", map[string]any{
		"Folders": h.cfg.Folders,
		"Error":   msg,
	})
}
