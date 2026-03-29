package handlers

import (
	"net/http"
	"path"

	"github.com/artogahr/simple-filestore/internal/storage"
)

// servePublicFile serves files without authentication at /{folder}/{rest...}.
// The folder name itself acts as the access key — if it's in the config and the
// file exists, it's served inline so the browser can render it natively.
func (h *Handler) servePublicFile(w http.ResponseWriter, r *http.Request) {
	folder := r.PathValue("folder")
	rest := r.PathValue("rest")

	// Folder must be registered in config
	if !h.cfg.HasFolder(folder) {
		http.NotFound(w, r)
		return
	}

	if rest == "" {
		// Bare /{folder}/ — redirect to the file browser
		http.Redirect(w, r, "/browse/", http.StatusSeeOther)
		return
	}

	f, info, err := h.store.Open(folder, rest)
	if err == storage.ErrNotFound {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	if info.IsDir() {
		http.NotFound(w, r)
		return
	}

	// Serve inline — let the browser decide how to render (PDF viewer,
	// video player, image display, etc.)
	w.Header().Set("Content-Disposition", `inline; filename="`+path.Base(rest)+`"`)
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}
