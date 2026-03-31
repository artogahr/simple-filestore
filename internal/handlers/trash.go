package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/artogahr/simple-filestore/internal/middleware"
	"github.com/artogahr/simple-filestore/internal/storage"
)

// TrashData is the template data for the trash page.
type TrashData struct {
	Folder  string
	Entries []storage.TrashEntry
}

func (h *Handler) trashPage(w http.ResponseWriter, r *http.Request) {
	folder := middleware.FolderFromContext(r.Context())
	entries, err := h.store.ListTrash(folder)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, fmt.Sprintf("Could not list trash: %v", err))
		return
	}
	data := TrashData{Folder: folder, Entries: entries}
	if isHTMX(r) {
		_ = h.tmpl.ExecuteTemplate(w, "trash-list", data)
		return
	}
	_ = h.tmpl.ExecuteTemplate(w, "trash.html", data)
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	folder := middleware.FolderFromContext(r.Context())
	id := r.FormValue("id")
	if id == "" {
		h.renderError(w, r, http.StatusBadRequest, "Missing item ID.")
		return
	}
	if err := h.store.RestoreFromTrash(folder, id); err != nil {
		h.renderError(w, r, http.StatusInternalServerError, fmt.Sprintf("Could not restore: %v", err))
		return
	}
	entries, _ := h.store.ListTrash(folder)
	data := TrashData{Folder: folder, Entries: entries}
	if isHTMX(r) {
		_ = h.tmpl.ExecuteTemplate(w, "trash-list", data)
		return
	}
	http.Redirect(w, r, "/trash", http.StatusSeeOther)
}

func (h *Handler) trashDelete(w http.ResponseWriter, r *http.Request) {
	folder := middleware.FolderFromContext(r.Context())
	id := strings.TrimPrefix(r.URL.Path, "/trash-item/")
	if id == "" {
		h.renderError(w, r, http.StatusBadRequest, "Missing item ID.")
		return
	}
	if err := h.store.PermanentDelete(folder, id); err != nil {
		h.renderError(w, r, http.StatusInternalServerError, fmt.Sprintf("Could not delete: %v", err))
		return
	}
	entries, _ := h.store.ListTrash(folder)
	data := TrashData{Folder: folder, Entries: entries}
	if isHTMX(r) {
		_ = h.tmpl.ExecuteTemplate(w, "trash-list", data)
		return
	}
	http.Redirect(w, r, "/trash", http.StatusSeeOther)
}
