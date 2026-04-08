package storage

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

var (
	// ErrSubscriptionExists повертається, якщо пара email + repo вже існує в базі
	ErrSubscriptionExists = errors.New("subscription already exists for this email and repository")
)

// SubscriptionStorage визначає методи для роботи з підписками
type SubscriptionStorage interface {
	CreateSubscription(email, repo string) error
	DeleteSubscription(id string) error
	GetActiveRepositories() ([]string, error)
	GetEmailsForRepo(repo string) ([]string, error)
	GetLastSeenTag(repo string) (string, error)
	UpdateLastSeenTag(repo, tag string) error
}

type postgresStorage struct {
	db *sql.DB
}

// NewPostgresStorage створює нову реалізацію SubscriptionStorage для PostgreSQL
func NewPostgresStorage(db *sql.DB) SubscriptionStorage {
	return &postgresStorage{
		db: db,
	}
}

// CreateSubscription створює нову підписку.
// Якщо пара (email, repo) вже існує, повертає ErrSubscriptionExists.
func (s *postgresStorage) CreateSubscription(email, repo string) error {
	query := `INSERT INTO subscriptions (email, repository) VALUES ($1, $2)`
	_, err := s.db.Exec(query, email, repo)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" { // unique_violation
			return ErrSubscriptionExists
		}
		return fmt.Errorf("failed to create subscription: %w", err)
	}
	return nil
}

// DeleteSubscription видаляє підписку за її UUID `id`.
func (s *postgresStorage) DeleteSubscription(id string) error {
	query := `DELETE FROM subscriptions WHERE id = $1`
	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete subscription %q: %w", id, err)
	}
	return nil
}

// GetActiveRepositories повертає унікальний список репозиторіїв з усіх підписок, де is_active = true
func (s *postgresStorage) GetActiveRepositories() ([]string, error) {
	query := `SELECT DISTINCT repository FROM subscriptions WHERE is_active = true`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query active repositories: %w", err)
	}
	defer rows.Close()

	var repos []string
	for rows.Next() {
		var repo string
		if err := rows.Scan(&repo); err != nil {
			return nil, fmt.Errorf("failed to scan repository row: %w", err)
		}
		repos = append(repos, repo)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return repos, nil
}

// GetEmailsForRepo повертає список email-ів усіх активних підписників для конкретного репозиторію
func (s *postgresStorage) GetEmailsForRepo(repo string) ([]string, error) {
	query := `SELECT email FROM subscriptions WHERE repository = $1 AND is_active = true`
	rows, err := s.db.Query(query, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to query emails for repo %q: %w", repo, err)
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("failed to scan email row: %w", err)
		}
		emails = append(emails, email)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("email rows iteration error: %w", err)
	}
	return emails, nil
}

// GetLastSeenTag повертає останній збережений тег для заданого репозиторію.
// Шукає перший-ліпший збережений запис, що має last_seen_tag.
func (s *postgresStorage) GetLastSeenTag(repo string) (string, error) {
	query := `SELECT last_seen_tag FROM subscriptions WHERE repository = $1 AND last_seen_tag IS NOT NULL LIMIT 1`
	var tag string
	err := s.db.QueryRow(query, repo).Scan(&tag)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil // ще не було збережено жодного тегу
		}
		return "", fmt.Errorf("failed to get last seen tag: %w", err)
	}
	return tag, nil
}

// UpdateLastSeenTag оновлює last_seen_tag для всіх підписок на цей репозиторій.
func (s *postgresStorage) UpdateLastSeenTag(repo, tag string) error {
	query := `UPDATE subscriptions SET last_seen_tag = $1 WHERE repository = $2`
	_, err := s.db.Exec(query, tag, repo)
	if err != nil {
		return fmt.Errorf("failed to update last seen tag for repo %q: %w", repo, err)
	}
	return nil
}
