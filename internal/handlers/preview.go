package handlers

import (
	"net/http"
	"path"
	"strings"

	"github.com/artogahr/simple-filestore/internal/middleware"
	"github.com/artogahr/simple-filestore/internal/storage"
)

const maxTextPreviewBytes = 1 << 20 // 1 MB

type previewData struct {
	Folder      string
	Path        string
	Name        string
	PreviewType string // "image", "video", "audio", "pdf", "text", "unsupported"
	ContentURL  string // URL to serve the file inline
	TextContent string // populated for text previews
	Truncated   bool   // text was truncated
	BackURL     string // parent directory URL
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	folder := middleware.FolderFromContext(r.Context())
	relPath := strings.TrimPrefix(r.URL.Path, "/preview/")

	pt := storage.PreviewType(path.Base(relPath))

	pd := previewData{
		Folder:      folder,
		Path:        relPath,
		Name:        path.Base(relPath),
		PreviewType: pt,
		ContentURL:  "/files/" + relPath + "?inline=1",
		BackURL:     backURL(relPath),
	}

	if pt == "text" {
		content, truncated, err := h.store.ReadText(folder, relPath, maxTextPreviewBytes)
		if err == nil {
			pd.TextContent = content
			pd.Truncated = truncated
		}
	}

	_ = h.tmpl.ExecuteTemplate(w, "preview.html", pd)
}

// backURL returns the browse URL for the parent directory of relPath.
func backURL(relPath string) string {
	parent := path.Dir(relPath)
	if parent == "." || parent == "" {
		return "/browse/"
	}
	return "/browse/" + parent
}
