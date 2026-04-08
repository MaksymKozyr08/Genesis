package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"genesis/internal/storage"

	"github.com/go-chi/chi/v5"
)

// Handler містить залежності для обробки HTTP запитів
type Handler struct {
	storage storage.SubscriptionStorage
}

// NewHandler створює новий екземпляр Handler
func NewHandler(s storage.SubscriptionStorage) *Handler {
	return &Handler{
		storage: s,
	}
}

// SubscribeRequest описує тіло запиту для підписки
type SubscribeRequest struct {
	Email      string `json:"email"`
	Repository string `json:"repository"`
}

// Subscribe обробляє POST /subscribe
func (h *Handler) Subscribe(w http.ResponseWriter, r *http.Request) {
	var req SubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Repository == "" {
		http.Error(w, "Email and repository are required", http.StatusBadRequest)
		return
	}

	// Перевірка формату owner/repo
	parts := strings.Split(req.Repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "Invalid repository format. Must be owner/repo", http.StatusBadRequest)
		return
	}

	// Перевірка існування репозиторію через GitHub API
	githubURL := fmt.Sprintf("https://api.github.com/repos/%s", req.Repository)
	resp, err := http.Get(githubURL)
	if err != nil {
		http.Error(w, "Failed to verify repository with GitHub API", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		http.Error(w, "Repository not found on GitHub", http.StatusNotFound)
		return
	} else if resp.StatusCode != http.StatusOK {
		http.Error(w, "Error communicating with GitHub API", http.StatusInternalServerError)
		return
	}

	// Створення підписки у БД
	err = h.storage.CreateSubscription(req.Email, req.Repository)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionExists) {
			http.Error(w, "Subscription already exists", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to save subscription in database", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status":"success"}`))
}

// Unsubscribe обробляє DELETE /subscribe/{id}
func (h *Handler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" { // Chi завжди парсить param, але залишаємо для підстраховки
		http.Error(w, "Missing subscription ID", http.StatusBadRequest)
		return
	}

	err := h.storage.DeleteSubscription(id)
	if err != nil {
		http.Error(w, "Failed to delete subscription", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
