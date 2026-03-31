package handlers

import (
	"fmt"
	"net/http"
	"path"
	"path/filepath"
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
	DiskTotal   uint64
	DiskAvail   uint64
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

	// Hide hidden entries (e.g. .trash) from the browser
	visible := entries[:0]
	for _, e := range entries {
		if !strings.HasPrefix(e.Name, ".") {
			visible = append(visible, e)
		}
	}
	entries = visible

	diskTotal, diskAvail, _ := h.store.DiskUsage()
	data := BrowseData{
		Folder:      folder,
		CurrentPath: relPath,
		Entries:     entries,
		Breadcrumbs: buildBreadcrumbs(folder, relPath),
		DiskTotal:   diskTotal,
		DiskAvail:   diskAvail,
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
	currentPath := r.FormValue("path")

	r.Body = http.MaxBytesReader(w, r.Body, 10<<30) // 10 GB
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Upload too large or malformed.")
		return
	}

	// relpath is set by the folder-upload JS to preserve directory structure.
	// e.g. relpath="my-docs/sub/file.txt", path="" → upload to sub/file.txt
	relpath := r.FormValue("relpath")

	file, header, err := r.FormFile("file")
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, "No file provided.")
		return
	}
	defer file.Close()

	var uploadDir, filename string
	if relpath != "" {
		// Folder upload: relpath carries the full relative path within the
		// selected folder (e.g. "docs/sub/report.pdf").
		// We join currentPath + the directory part of relpath.
		dir := filepath.Dir(filepath.ToSlash(relpath))
		if dir == "." {
			dir = ""
		}
		uploadDir = joinPathStr(currentPath, dir)
		filename = filepath.Base(relpath)
	} else {
		uploadDir = currentPath
		filename = header.Filename
	}

	if err := h.store.Upload(folder, uploadDir, filename, file); err != nil {
		h.renderError(w, r, http.StatusInternalServerError, fmt.Sprintf("Upload failed: %v", err))
		return
	}

	if isHTMX(r) {
		entries, _ := h.store.List(folder, currentPath)
		data := BrowseData{
			Folder:      folder,
			CurrentPath: currentPath,
			Entries:     entries,
			Breadcrumbs: buildBreadcrumbs(folder, currentPath),
		}
		_ = h.tmpl.ExecuteTemplate(w, "file-list", data)
		return
	}
	http.Redirect(w, r, "/browse/"+currentPath, http.StatusSeeOther)
}

// zipDownload streams a ZIP archive of folder/relPath to the client.
func (h *Handler) zipDownload(w http.ResponseWriter, r *http.Request) {
	folder := middleware.FolderFromContext(r.Context())
	relPath := r.URL.Query().Get("path")

	// Determine ZIP filename from the directory being zipped
	zipName := folder
	if relPath != "" {
		zipName = path.Base(relPath)
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, zipName))

	if err := h.store.StreamZip(folder, relPath, w); err != nil {
		// Can't write a proper error response after headers are sent
		return
	}
}

// joinPathStr joins path segments, skipping empty ones.
func joinPathStr(parts ...string) string {
	var sb strings.Builder
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte('/')
		}
		sb.WriteString(p)
	}
	return sb.String()
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
