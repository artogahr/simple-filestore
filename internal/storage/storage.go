// Package storage handles all filesystem operations for simple-filestore.
// All paths are validated through safeJoin to prevent path traversal attacks.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrOutsideRoot is returned when a computed path escapes the folder root.
var ErrOutsideRoot = errors.New("path escapes folder root")

// ErrNotFound is returned when the requested file or folder does not exist.
var ErrNotFound = errors.New("not found")

// Store manages the workspace/folders directory.
type Store struct {
	root string // absolute path to workspace/folders/
}

// New creates a Store rooted at the given path (workspace/folders/).
// It creates the directory if it does not exist.
func New(root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0750); err != nil {
		return nil, err
	}
	return &Store{root: abs}, nil
}

// --- Entry type ---------------------------------------------------------------

// Entry represents a file or directory in a listing.
type Entry struct {
	Name     string
	IsDir    bool
	Size     int64
	ModTime  time.Time
	MIMEType string
}

// --- Path safety --------------------------------------------------------------

// safeJoin joins root/folder/parts, cleans the result, and verifies it still
// lives under root/folder. Returns ErrOutsideRoot if the path escapes.
func (s *Store) safeJoin(folder string, parts ...string) (string, error) {
	// Validate folder name itself
	if err := validateFolderName(folder); err != nil {
		return "", err
	}
	folderRoot := filepath.Join(s.root, folder)

	all := append([]string{folderRoot}, parts...)
	joined := filepath.Join(all...)
	cleaned := filepath.Clean(joined)

	prefix := folderRoot + string(os.PathSeparator)
	if cleaned != folderRoot && !strings.HasPrefix(cleaned, prefix) {
		return "", ErrOutsideRoot
	}
	return cleaned, nil
}

// folderRoot returns the absolute path to a folder, validating the name.
func (s *Store) folderRoot(folder string) (string, error) {
	if err := validateFolderName(folder); err != nil {
		return "", err
	}
	return filepath.Join(s.root, folder), nil
}

// trashRoot returns the absolute path to a folder's trash directory.
func (s *Store) trashRoot(folder string) (string, error) {
	root, err := s.folderRoot(folder)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".trash"), nil
}

// validateFolderName checks that a folder name is safe (no traversal, no dots).
func validateFolderName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid folder name: %q", name)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid folder name: %q", name)
	}
	return nil
}

// validateFileName checks that an uploaded filename is safe.
func validateFileName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid filename: %q", name)
	}
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("invalid filename: contains null byte")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid filename: contains path separator: %q", name)
	}
	return nil
}

// --- Listing ------------------------------------------------------------------

// List returns the contents of relPath within folder.
// relPath = "" means the folder root. Hidden entries (starting with ".") are
// excluded from the listing (this hides .trash and other internals).
func (s *Store) List(folder, relPath string) ([]Entry, error) {
	var target string
	var err error
	if relPath == "" {
		target, err = s.safeJoin(folder)
	} else {
		target, err = s.safeJoin(folder, relPath)
	}
	if err != nil {
		return nil, err
	}

	des, err := os.ReadDir(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var entries []Entry
	for _, de := range des {
		name := de.Name()
		// Skip hidden entries (trash, etc.)
		if strings.HasPrefix(name, ".") {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		e := Entry{
			Name:    name,
			IsDir:   de.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		if !de.IsDir() {
			e.MIMEType = mimeForName(name)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// --- File access --------------------------------------------------------------

// Open returns an *os.File for reading (supports seeking for http.ServeContent).
// Caller must close the file.
func (s *Store) Open(folder, relPath string) (*os.File, os.FileInfo, error) {
	target, err := s.safeJoin(folder, relPath)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, info, nil
}

// ReadText reads up to maxBytes of a text file and returns its content.
func (s *Store) ReadText(folder, relPath string, maxBytes int64) (string, bool, error) {
	f, _, err := s.Open(folder, relPath)
	if err != nil {
		return "", false, err
	}
	defer f.Close()

	r := io.LimitReader(f, maxBytes+1)
	data, err := io.ReadAll(r)
	if err != nil {
		return "", false, err
	}
	truncated := int64(len(data)) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}
	return string(data), truncated, nil
}

// --- Mutating operations ------------------------------------------------------

// Upload streams r to folder/relPath/filename. It creates intermediate
// directories as needed. filename is sanitized with filepath.Base before use.
func (s *Store) Upload(folder, relPath, filename string, r io.Reader) error {
	filename = filepath.Base(filename) // strip any directory components from client
	if err := validateFileName(filename); err != nil {
		return err
	}

	var dir string
	var err error
	if relPath == "" {
		dir, err = s.safeJoin(folder)
	} else {
		dir, err = s.safeJoin(folder, relPath)
	}
	if err != nil {
		return err
	}

	destPath := filepath.Join(dir, filename)
	// Verify the final dest path is still within folder root
	folderRoot := filepath.Join(s.root, folder)
	if !strings.HasPrefix(destPath, folderRoot+string(os.PathSeparator)) {
		return ErrOutsideRoot
	}

	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, r)
	return err
}

// MakeDir creates a directory at folder/relPath/name.
func (s *Store) MakeDir(folder, relPath, name string) error {
	if err := validateFileName(name); err != nil {
		return err
	}
	var base string
	var err error
	if relPath == "" {
		base, err = s.safeJoin(folder)
	} else {
		base, err = s.safeJoin(folder, relPath)
	}
	if err != nil {
		return err
	}
	target := filepath.Join(base, name)
	folderRoot := filepath.Join(s.root, folder)
	if !strings.HasPrefix(target, folderRoot+string(os.PathSeparator)) {
		return ErrOutsideRoot
	}
	return os.MkdirAll(target, 0750)
}

// Rename renames a file or directory within the same folder.
// oldPath and newName are both relative to the folder root.
// newName is just the new base name (no directory change).
func (s *Store) Rename(folder, oldPath, newName string) error {
	if err := validateFileName(newName); err != nil {
		return err
	}
	src, err := s.safeJoin(folder, oldPath)
	if err != nil {
		return err
	}
	// New path is same directory, different name
	dst := filepath.Join(filepath.Dir(src), newName)
	folderRoot := filepath.Join(s.root, folder)
	if !strings.HasPrefix(dst, folderRoot+string(os.PathSeparator)) {
		return ErrOutsideRoot
	}
	return os.Rename(src, dst)
}

// --- Trash operations ---------------------------------------------------------

// TrashEntry describes an item in the trash.
type TrashEntry struct {
	ID           string    `json:"id"`
	OriginalPath string    `json:"original_path"` // relative to folder root
	Name         string    `json:"name"`
	IsDir        bool      `json:"is_dir"`
	Size         int64     `json:"size"`
	DeletedAt    time.Time `json:"deleted_at"`
}

// MoveToTrash moves folder/relPath to the folder's trash with a unique ID.
func (s *Store) MoveToTrash(folder, relPath string) error {
	src, err := s.safeJoin(folder, relPath)
	if err != nil {
		return err
	}
	info, err := os.Lstat(src)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	trashDir, err := s.trashRoot(folder)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(trashDir, 0750); err != nil {
		return err
	}

	id := newID()
	dst := filepath.Join(trashDir, id)
	if err := os.Rename(src, dst); err != nil {
		return err
	}

	meta := TrashEntry{
		ID:           id,
		OriginalPath: relPath,
		Name:         filepath.Base(relPath),
		IsDir:        info.IsDir(),
		Size:         info.Size(),
		DeletedAt:    time.Now().UTC(),
	}
	return writeTrashMeta(trashDir, id, meta)
}

// ListTrash returns all items in a folder's trash.
func (s *Store) ListTrash(folder string) ([]TrashEntry, error) {
	trashDir, err := s.trashRoot(folder)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(trashDir); os.IsNotExist(err) {
		return nil, nil
	}

	des, err := os.ReadDir(trashDir)
	if err != nil {
		return nil, err
	}

	var entries []TrashEntry
	for _, de := range des {
		if !strings.HasSuffix(de.Name(), ".meta") {
			continue
		}
		id := strings.TrimSuffix(de.Name(), ".meta")
		meta, err := readTrashMeta(trashDir, id)
		if err != nil {
			continue // skip corrupt meta
		}
		entries = append(entries, meta)
	}
	return entries, nil
}

// RestoreFromTrash moves the item with the given ID back to its original path.
func (s *Store) RestoreFromTrash(folder, id string) error {
	trashDir, err := s.trashRoot(folder)
	if err != nil {
		return err
	}
	meta, err := readTrashMeta(trashDir, id)
	if err != nil {
		return fmt.Errorf("trash item not found: %w", err)
	}

	src := filepath.Join(trashDir, id)
	dst, err := s.safeJoin(folder, meta.OriginalPath)
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
		return err
	}

	if err := os.Rename(src, dst); err != nil {
		return err
	}
	return os.Remove(filepath.Join(trashDir, id+".meta"))
}

// PermanentDelete permanently deletes the trash item with the given ID.
func (s *Store) PermanentDelete(folder, id string) error {
	trashDir, err := s.trashRoot(folder)
	if err != nil {
		return err
	}
	_, err = readTrashMeta(trashDir, id)
	if err != nil {
		return fmt.Errorf("trash item not found: %w", err)
	}

	item := filepath.Join(trashDir, id)
	if err := os.RemoveAll(item); err != nil {
		return err
	}
	return os.Remove(filepath.Join(trashDir, id+".meta"))
}

// --- Folder management (admin) ------------------------------------------------

// FolderExists reports whether the folder directory exists on disk.
func (s *Store) FolderExists(name string) bool {
	if err := validateFolderName(name); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(s.root, name))
	return err == nil
}

// CreateFolder creates the folder directory and its trash subdirectory.
func (s *Store) CreateFolder(name string) error {
	if err := validateFolderName(name); err != nil {
		return err
	}
	path := filepath.Join(s.root, name)
	if err := os.MkdirAll(path, 0750); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(path, ".trash"), 0750)
}

// DeleteFolder moves the folder to workspace/deleted/<name>-<timestamp>
// instead of permanently removing it.
func (s *Store) DeleteFolder(deletedRoot, name string) error {
	if err := validateFolderName(name); err != nil {
		return err
	}
	src := filepath.Join(s.root, name)
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return ErrNotFound
	}

	if err := os.MkdirAll(deletedRoot, 0750); err != nil {
		return err
	}

	dst := filepath.Join(deletedRoot, fmt.Sprintf("%s-%d", name, time.Now().Unix()))
	return os.Rename(src, dst)
}

// --- Helpers ------------------------------------------------------------------

func writeTrashMeta(trashDir, id string, meta TrashEntry) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(trashDir, id+".meta"), data, 0640)
}

func readTrashMeta(trashDir, id string) (TrashEntry, error) {
	data, err := os.ReadFile(filepath.Join(trashDir, id+".meta"))
	if err != nil {
		return TrashEntry{}, err
	}
	var meta TrashEntry
	if err := json.Unmarshal(data, &meta); err != nil {
		return TrashEntry{}, err
	}
	return meta, nil
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// mimeForName returns a MIME type based on the file extension.
func mimeForName(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if m := mime.TypeByExtension(ext); m != "" {
		return m
	}
	// Fallback for common types not always registered
	switch ext {
	case ".md":
		return "text/markdown"
	case ".go":
		return "text/x-go"
	case ".py":
		return "text/x-python"
	case ".rs":
		return "text/x-rust"
	case ".ts":
		return "text/typescript"
	case ".tsx":
		return "text/typescript"
	case ".jsx":
		return "text/javascript"
	case ".sh":
		return "text/x-sh"
	case ".toml":
		return "text/x-toml"
	case ".yaml", ".yml":
		return "text/yaml"
	}
	return "application/octet-stream"
}

// PreviewType determines what kind of preview to show for a file.
func PreviewType(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".avif", ".bmp", ".ico":
		return "image"
	case ".mp4", ".webm", ".ogg", ".mov", ".mkv", ".avi":
		return "video"
	case ".mp3", ".flac", ".wav", ".m4a", ".aac", ".opus":
		return "audio"
	case ".pdf":
		return "pdf"
	case ".txt", ".md", ".csv", ".log", ".json", ".xml",
		".yaml", ".yml", ".toml", ".ini", ".env",
		".go", ".py", ".js", ".ts", ".jsx", ".tsx",
		".sh", ".bash", ".zsh", ".fish",
		".rs", ".c", ".h", ".cpp", ".hpp", ".java",
		".html", ".htm", ".css", ".scss",
		".sql", ".nix", ".conf", ".cfg":
		return "text"
	default:
		return "unsupported"
	}
}

// DetectContentType sniffs the MIME type from a file's first 512 bytes.
func DetectContentType(f *os.File) string {
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	f.Seek(0, io.SeekStart)
	return http.DetectContentType(buf[:n])
}
