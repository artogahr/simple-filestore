package main

import (
	"flag"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artogahr/simple-filestore/internal/assets"
	"github.com/artogahr/simple-filestore/internal/config"
	"github.com/artogahr/simple-filestore/internal/handlers"
	"github.com/artogahr/simple-filestore/internal/middleware"
	"github.com/artogahr/simple-filestore/internal/storage"
)

func main() {
	var (
		workspaceDir = flag.String("workspace", "./workspace", "path to workspace directory")
		portFlag     = flag.Int("port", 0, "port to listen on (overrides config)")
	)
	flag.Parse()

	// Ensure workspace structure exists
	ws := *workspaceDir
	for _, dir := range []string{
		filepath.Join(ws, "folders"),
		filepath.Join(ws, "deleted"),
	} {
		if err := os.MkdirAll(dir, 0750); err != nil {
			slog.Error("failed to create workspace directory", "dir", dir, "err", err)
			os.Exit(1)
		}
	}

	// Load config
	cfgPath := filepath.Join(ws, "config.json")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("failed to load config", "path", cfgPath, "err", err)
		os.Exit(1)
	}
	if *portFlag > 0 {
		cfg.Port = *portFlag
	}

	// Initialize storage
	store, err := storage.New(filepath.Join(ws, "folders"))
	if err != nil {
		slog.Error("failed to initialize storage", "err", err)
		os.Exit(1)
	}

	// Initialize auth
	auth, err := middleware.New(cfg.SecretKey)
	if err != nil {
		slog.Error("failed to initialize auth", "err", err)
		os.Exit(1)
	}

	// Parse templates
	funcMap := template.FuncMap{
		"humanSize":  humanSize,
		"formatTime": formatTime,
		"joinPath":   joinPath,
		"isImage":    storage.IsImage,
		"sub":        func(a, b int) int { return a - b },
	}

	tmpl, err := template.New("base.html").Funcs(funcMap).ParseFS(assets.FS, "templates/*.html")
	if err != nil {
		slog.Error("failed to parse templates", "err", err)
		os.Exit(1)
	}

	// Build handler and routes
	deletedDir := filepath.Join(ws, "deleted")
	h := handlers.New(cfg, cfgPath, store, auth, tmpl, deletedDir)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Static files
	mux.Handle("/static/", http.FileServer(http.FS(assets.FS)))

	// Wrap with logging middleware
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: loggingMiddleware(mux),
	}

	slog.Info("starting simple-filestore", "addr", srv.Addr, "workspace", ws)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

// loggingMiddleware logs each request with method, path, status, and duration.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration", time.Since(start).String(),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// --- Template functions -------------------------------------------------------

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatTime(t time.Time) string {
	now := time.Now()
	if t.Year() == now.Year() {
		return t.Format("Jan 2, 15:04")
	}
	return t.Format("Jan 2, 2006")
}

func joinPath(parts ...string) string {
	var sb strings.Builder
	for i, p := range parts {
		p = strings.Trim(p, "/")
		if p == "" {
			continue
		}
		if i > 0 && sb.Len() > 0 {
			sb.WriteByte('/')
		}
		sb.WriteString(p)
	}
	return sb.String()
}
