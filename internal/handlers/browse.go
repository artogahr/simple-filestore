package handlers

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/artogahr/simple-filestore/internal/middleware"
	"github.com/artogahr/simple-filestore/internal/storage"
)

// Breadcrumb is a navigation element for the file browser.
type Breadcrumb struct {
	Name string
	URL  string
}

// BrowseData is the template data for the file browser page.
type BrowseData struct {
	Folder      string
	CurrentPath string // relative path within the folder, "" for root
	Entries     []storage.Entry
	Breadcrumbs []Breadcrumb
}

func (h *Handler) browseRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/browse/", http.StatusSeeOther)
}

func (h *Handler) browse(w http.ResponseWriter, r *http.Request) {
	folder := middleware.FolderFromContext(r.Context())
	// Extract path after /browse/
	relPath := strings.TrimPrefix(r.URL.Path, "/browse/")
	relPath = strings.TrimSuffix(relPath, "/")

	entries, err := h.store.List(folder, relPath)
	if err == storage.ErrNotFound {
		h.renderError(w, r, http.StatusNotFound, "Directory not found.")
		return
	}
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, fmt.Sprintf("Could not list directory: %v", err))
		return
	}

	data := BrowseData{
		Folder:      folder,
		CurrentPath: relPath,
		Entries:     entries,
		Breadcrumbs: buildBreadcrumbs(folder, relPath),
	}

	if isHTMX(r) {
		_ = h.tmpl.ExecuteTemplate(w, "file-list", data)
		return
	}
	_ = h.tmpl.ExecuteTemplate(w, "browser.html", data)
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	folder := middleware.FolderFromContext(r.Context())
	relPath := strings.TrimPrefix(r.URL.Path, "/files/")

	f, info, err := h.store.Open(folder, relPath)
	if err == storage.ErrNotFound {
		h.renderError(w, r, http.StatusNotFound, "File not found.")
		return
	}
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "Could not open file.")
		return
	}
	defer f.Close()

	inline := r.URL.Query().Get("inline") == "1"
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename=%q`, disposition, path.Base(relPath)))

	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	folder := middleware.FolderFromContext(r.Context())
	relPath := r.FormValue("path")

	r.Body = http.MaxBytesReader(w, r.Body, 10<<30) // 10 GB
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Upload too large or malformed.")
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		file, header, err := r.FormFile("file")
		if err != nil {
			h.renderError(w, r, http.StatusBadRequest, "No file provided.")
			return
		}
		defer file.Close()
		if err := h.store.Upload(folder, relPath, header.Filename, file); err != nil {
			h.renderError(w, r, http.StatusInternalServerError, fmt.Sprintf("Upload failed: %v", err))
			return
		}
	} else {
		for _, fh := range files {
			f, err := fh.Open()
			if err != nil {
				continue
			}
			h.store.Upload(folder, relPath, fh.Filename, f)
			f.Close()
		}
	}

	if isHTMX(r) {
		// Refresh the file list
		entries, _ := h.store.List(folder, relPath)
		data := BrowseData{
			Folder:      folder,
			CurrentPath: relPath,
			Entries:     entries,
			Breadcrumbs: buildBreadcrumbs(folder, relPath),
		}
		w.Header().Set("HX-Trigger", "fileListRefresh")
		_ = h.tmpl.ExecuteTemplate(w, "file-list", data)
		return
	}
	target := "/browse/" + relPath
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (h *Handler) mkdir(w http.ResponseWriter, r *http.Request) {
	folder := middleware.FolderFromContext(r.Context())
	relPath := r.FormValue("path")
	name := strings.TrimSpace(r.FormValue("name"))

	if name == "" {
		h.renderError(w, r, http.StatusBadRequest, "Folder name is required.")
		return
	}
	if err := h.store.MakeDir(folder, relPath, name); err != nil {
		h.renderError(w, r, http.StatusBadRequest, fmt.Sprintf("Could not create folder: %v", err))
		return
	}

	if isHTMX(r) {
		entries, _ := h.store.List(folder, relPath)
		data := BrowseData{
			Folder:      folder,
			CurrentPath: relPath,
			Entries:     entries,
			Breadcrumbs: buildBreadcrumbs(folder, relPath),
		}
		_ = h.tmpl.ExecuteTemplate(w, "file-list", data)
		return
	}
	http.Redirect(w, r, "/browse/"+relPath, http.StatusSeeOther)
}

func (h *Handler) rename(w http.ResponseWriter, r *http.Request) {
	folder := middleware.FolderFromContext(r.Context())
	oldPath := r.FormValue("path")
	newName := strings.TrimSpace(r.FormValue("name"))

	if newName == "" {
		h.renderError(w, r, http.StatusBadRequest, "New name is required.")
		return
	}
	if err := h.store.Rename(folder, oldPath, newName); err != nil {
		h.renderError(w, r, http.StatusBadRequest, fmt.Sprintf("Could not rename: %v", err))
		return
	}

	// Respond with the parent directory listing
	parentPath := path.Dir(oldPath)
	if parentPath == "." {
		parentPath = ""
	}

	if isHTMX(r) {
		entries, _ := h.store.List(folder, parentPath)
		data := BrowseData{
			Folder:      folder,
			CurrentPath: parentPath,
			Entries:     entries,
			Breadcrumbs: buildBreadcrumbs(folder, parentPath),
		}
		_ = h.tmpl.ExecuteTemplate(w, "file-list", data)
		return
	}
	http.Redirect(w, r, "/browse/"+parentPath, http.StatusSeeOther)
}

func (h *Handler) deleteFile(w http.ResponseWriter, r *http.Request) {
	folder := middleware.FolderFromContext(r.Context())
	relPath := strings.TrimPrefix(r.URL.Path, "/files/")

	if err := h.store.MoveToTrash(folder, relPath); err == storage.ErrNotFound {
		h.renderError(w, r, http.StatusNotFound, "File not found.")
		return
	} else if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, fmt.Sprintf("Could not delete: %v", err))
		return
	}

	parentPath := path.Dir(relPath)
	if parentPath == "." {
		parentPath = ""
	}

	if isHTMX(r) {
		entries, _ := h.store.List(folder, parentPath)
		data := BrowseData{
			Folder:      folder,
			CurrentPath: parentPath,
			Entries:     entries,
			Breadcrumbs: buildBreadcrumbs(folder, parentPath),
		}
		_ = h.tmpl.ExecuteTemplate(w, "file-list", data)
		return
	}
	http.Redirect(w, r, "/browse/"+parentPath, http.StatusSeeOther)
}

// buildBreadcrumbs constructs navigation breadcrumbs for the given folder/path.
func buildBreadcrumbs(folder, relPath string) []Breadcrumb {
	crumbs := []Breadcrumb{
		{Name: folder, URL: "/browse/"},
	}
	if relPath == "" {
		return crumbs
	}
	parts := strings.Split(relPath, "/")
	for i, part := range parts {
		if part == "" {
			continue
		}
		url := "/browse/" + strings.Join(parts[:i+1], "/")
		crumbs = append(crumbs, Breadcrumb{Name: part, URL: url})
	}
	return crumbs
}
