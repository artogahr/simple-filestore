package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// --- safeJoin tests -----------------------------------------------------------

func TestSafeJoin_normalPath(t *testing.T) {
	s := newTestStore(t)
	// Create folder directory
	os.MkdirAll(filepath.Join(s.root, "myfolder"), 0750)

	got, err := s.safeJoin("myfolder", "subdir", "file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(s.root, "myfolder", "subdir", "file.txt")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSafeJoin_traversal(t *testing.T) {
	s := newTestStore(t)
	os.MkdirAll(filepath.Join(s.root, "myfolder"), 0750)

	cases := []string{
		"../other",
		"../../etc/passwd",
		"subdir/../../other",
		"subdir/../../../etc",
	}
	for _, c := range cases {
		_, err := s.safeJoin("myfolder", c)
		if err == nil {
			t.Errorf("safeJoin(%q) should have returned error", c)
		}
	}
}

func TestSafeJoin_folderRoot(t *testing.T) {
	s := newTestStore(t)
	os.MkdirAll(filepath.Join(s.root, "myfolder"), 0750)

	// Empty relPath should return folder root
	got, err := s.safeJoin("myfolder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(s.root, "myfolder")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSafeJoin_invalidFolderName(t *testing.T) {
	s := newTestStore(t)
	_, err := s.safeJoin("../etc")
	if err == nil {
		t.Error("expected error for traversal folder name")
	}
}

// --- validateFileName tests ---------------------------------------------------

func TestValidateFileName(t *testing.T) {
	good := []string{"file.txt", "my-file.pdf", "image (1).jpg", "data_2026.csv"}
	for _, name := range good {
		if err := validateFileName(name); err != nil {
			t.Errorf("validateFileName(%q) = %v, want nil", name, err)
		}
	}

	bad := []string{".", "..", "/etc/passwd", "a/b", "a\\b", "null\x00byte"}
	for _, name := range bad {
		if err := validateFileName(name); err == nil {
			t.Errorf("validateFileName(%q) = nil, want error", name)
		}
	}
}

// --- List tests ---------------------------------------------------------------

func TestList_hidesTrash(t *testing.T) {
	s := newTestStore(t)
	folder := "testfolder"
	root := filepath.Join(s.root, folder)
	os.MkdirAll(filepath.Join(root, ".trash"), 0750)
	os.WriteFile(filepath.Join(root, "visible.txt"), []byte("hi"), 0640)

	entries, err := s.List(folder, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name, ".") {
			t.Errorf("List returned hidden entry %q", e.Name)
		}
	}
	if len(entries) != 1 || entries[0].Name != "visible.txt" {
		t.Errorf("expected [visible.txt], got %+v", entries)
	}
}

// --- Trash tests --------------------------------------------------------------

func TestTrashAndRestore(t *testing.T) {
	s := newTestStore(t)
	folder := "testfolder"
	root := filepath.Join(s.root, folder)
	os.MkdirAll(root, 0750)

	// Create a file and move it to trash
	filePath := "document.txt"
	os.WriteFile(filepath.Join(root, filePath), []byte("content"), 0640)

	if err := s.MoveToTrash(folder, filePath); err != nil {
		t.Fatalf("MoveToTrash: %v", err)
	}

	// Original should be gone
	if _, err := os.Stat(filepath.Join(root, filePath)); !os.IsNotExist(err) {
		t.Error("file should be gone after MoveToTrash")
	}

	// List trash
	items, err := s.ListTrash(folder)
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 trash item, got %d", len(items))
	}
	if items[0].Name != "document.txt" {
		t.Errorf("trash item name = %q, want document.txt", items[0].Name)
	}

	// Restore
	if err := s.RestoreFromTrash(folder, items[0].ID); err != nil {
		t.Fatalf("RestoreFromTrash: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, filePath)); err != nil {
		t.Error("file should be back after restore")
	}

	// Trash should be empty
	items, _ = s.ListTrash(folder)
	if len(items) != 0 {
		t.Errorf("expected empty trash after restore, got %d items", len(items))
	}
}

func TestPermanentDelete(t *testing.T) {
	s := newTestStore(t)
	folder := "testfolder"
	root := filepath.Join(s.root, folder)
	os.MkdirAll(root, 0750)
	os.WriteFile(filepath.Join(root, "bye.txt"), []byte("bye"), 0640)

	s.MoveToTrash(folder, "bye.txt")
	items, _ := s.ListTrash(folder)
	if len(items) != 1 {
		t.Fatal("expected 1 trash item")
	}

	if err := s.PermanentDelete(folder, items[0].ID); err != nil {
		t.Fatalf("PermanentDelete: %v", err)
	}
	items, _ = s.ListTrash(folder)
	if len(items) != 0 {
		t.Errorf("expected empty trash after permanent delete")
	}
}

// --- PreviewType tests --------------------------------------------------------

func TestPreviewType(t *testing.T) {
	cases := map[string]string{
		"photo.jpg":    "image",
		"clip.mp4":    "video",
		"song.mp3":    "audio",
		"doc.pdf":     "pdf",
		"readme.md":   "text",
		"main.go":     "text",
		"archive.zip": "unsupported",
		"data.bin":    "unsupported",
	}
	for name, want := range cases {
		if got := PreviewType(name); got != want {
			t.Errorf("PreviewType(%q) = %q, want %q", name, got, want)
		}
	}
}
