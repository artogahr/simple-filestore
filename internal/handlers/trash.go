package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/artogahr/simple-filestore/internal/middleware"
)

func (h *Handler) trashList(w http.ResponseWriter, r *http.Request) {
	folder := middleware.FolderFromContext(r.Context())
	entries, err := h.store.ListTrash(folder)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, fmt.Sprintf("Could not list trash: %v", err))
		return
	}
	_ = h.tmpl.ExecuteTemplate(w, "trash.html", map[string]any{
		"Folder":  folder,
		"Entries": entries,
	})
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	folder := middleware.FolderFromContext(r.Context())
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		h.renderError(w, r, http.StatusBadRequest, "Missing item ID.")
		return
	}
	if err := h.store.RestoreFromTrash(folder, id); err != nil {
		h.renderError(w, r, http.StatusInternalServerError, fmt.Sprintf("Could not restore: %v", err))
		return
	}

	if isHTMX(r) {
		entries, _ := h.store.ListTrash(folder)
		_ = h.tmpl.ExecuteTemplate(w, "trash-list", map[string]any{
			"Folder":  folder,
			"Entries": entries,
		})
		return
	}
	http.Redirect(w, r, "/trash", http.StatusSeeOther)
}

func (h *Handler) permanentDelete(w http.ResponseWriter, r *http.Request) {
	folder := middleware.FolderFromContext(r.Context())
	id := strings.TrimPrefix(r.URL.Path, "/trash-item/")

	if err := h.store.PermanentDelete(folder, id); err != nil {
		h.renderError(w, r, http.StatusInternalServerError, fmt.Sprintf("Could not delete: %v", err))
		return
	}

	if isHTMX(r) {
		entries, _ := h.store.ListTrash(folder)
		_ = h.tmpl.ExecuteTemplate(w, "trash-list", map[string]any{
			"Folder":  folder,
			"Entries": entries,
		})
		return
	}
	http.Redirect(w, r, "/trash", http.StatusSeeOther)
}
