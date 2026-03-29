package handlers

import (
	"net/http"
	"strings"
)

func (h *Handler) loginPage(w http.ResponseWriter, r *http.Request) {
	if folder, ok := h.auth.GetUserSession(r); ok && folder != "" {
		http.Redirect(w, r, "/browse/", http.StatusSeeOther)
		return
	}
	_ = h.tmpl.ExecuteTemplate(w, "login.html", nil)
}

func (h *Handler) postLogin(w http.ResponseWriter, r *http.Request) {
	folder := strings.TrimSpace(r.FormValue("folder"))
	if folder == "" {
		_ = h.tmpl.ExecuteTemplate(w, "login.html", map[string]any{"Error": "Please enter a folder name."})
		return
	}
	if !h.cfg.HasFolder(folder) {
		_ = h.tmpl.ExecuteTemplate(w, "login.html", map[string]any{"Error": "Folder not found."})
		return
	}
	if err := h.auth.SetUserSession(w, folder); err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "Could not create session.")
		return
	}
	http.Redirect(w, r, "/browse/", http.StatusSeeOther)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	h.auth.ClearUserSession(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) adminLoginPage(w http.ResponseWriter, r *http.Request) {
	if h.auth.IsAdmin(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	_ = h.tmpl.ExecuteTemplate(w, "admin_login.html", nil)
}

func (h *Handler) postAdminLogin(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")
	if password != h.cfg.AdminPassword {
		_ = h.tmpl.ExecuteTemplate(w, "admin_login.html", map[string]any{"Error": "Invalid password."})
		return
	}
	if err := h.auth.SetAdminSession(w); err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "Could not create session.")
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *Handler) adminLogout(w http.ResponseWriter, r *http.Request) {
	h.auth.ClearAdminSession(w)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}
