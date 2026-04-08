package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"genesis/internal/storage"
)

// MockStorage є ручною мок-реалізацією інтерфейсу SubscriptionStorage
type MockStorage struct {
	CreateSubscriptionFunc func(email, repo string) error
}

func (m *MockStorage) CreateSubscription(email, repo string) error {
	if m.CreateSubscriptionFunc != nil {
		return m.CreateSubscriptionFunc(email, repo)
	}
	return nil
}

func (m *MockStorage) DeleteSubscription(id string) error { return nil }
func (m *MockStorage) GetActiveRepositories() ([]string, error) { return nil, nil }
func (m *MockStorage) GetEmailsForRepo(repo string) ([]string, error) { return nil, nil }
func (m *MockStorage) GetLastSeenTag(repo string) (string, error) { return "", nil }
func (m *MockStorage) UpdateLastSeenTag(repo, tag string) error { return nil }

func TestSubscribe(t *testing.T) {
	tests := []struct {
		name           string
		payload        string
		mockCreate     func(email, repo string) error
		expectedStatus int
	}{
		{
			name:           "Validation Error - Missing Body",
			payload:        `{}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Validation Error - Invalid Format",
			payload:        `{"email":"test@example.com", "repository":"invalid_format"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:    "Conflict - Already Exists",
			payload: `{"email":"test@example.com", "repository":"golang/go"}`,
			mockCreate: func(email, repo string) error {
				return storage.ErrSubscriptionExists
			},
			expectedStatus: http.StatusConflict,
		},
		{
			name:    "Success - Created",
			// Цей тест робить реальний запит на GitHub API до golang/go
			payload: `{"email":"new@example.com", "repository":"golang/go"}`,
			mockCreate: func(email, repo string) error {
				return nil
			},
			expectedStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &MockStorage{
				CreateSubscriptionFunc: tt.mockCreate,
			}
			h := NewHandler(mockStore)

			req, err := http.NewRequest(http.MethodPost, "/subscribe", bytes.NewBuffer([]byte(tt.payload)))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			h.Subscribe(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}
