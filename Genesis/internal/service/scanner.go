package service

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"genesis/internal/storage"
)

// Scanner відповідає за фонове опитування репозиторіїв GitHub
type Scanner struct {
	storage     storage.SubscriptionStorage
	notifier    Notifier
	interval    time.Duration
	githubToken string
}

// NewScanner створює новий екземпляр Scanner
func NewScanner(s storage.SubscriptionStorage, n Notifier, interval time.Duration, githubToken string) *Scanner {
	return &Scanner{
		storage:     s,
		notifier:    n,
		interval:    interval,
		githubToken: githubToken,
	}
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

// Start запускає нескінченний цикл опитування у горутині
func (s *Scanner) Start() {
	go func() {
		log.Println("[SCANNER] Фоновий воркер запущено. Інтервал опитування:", s.interval)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		// Робимо первинне опитування одразу при запуску
		s.scan()

		for {
			<-ticker.C
			s.scan()
		}
	}()
}

func (s *Scanner) scan() {
	repos, err := s.storage.GetActiveRepositories()
	if err != nil {
		log.Printf("[SCANNER] Помилка отримання списку репозиторіїв: %v\n", err)
		return
	}

	for _, repo := range repos {
		if err := s.checkRepo(repo); err != nil {
			log.Printf("[SCANNER] Помилка перевірки репозиторію %s: %v\n", repo, err)
			
			// Відповідно до ТЗ: якщо отримали 429 - перериваємо поточний цикл перевірки
			if err.Error() == "rate limit exceeded" {
				return
			}
		}
	}
}

// checkRepo перевіряє конкретний репозиторій на наявність нових релізів
func (s *Scanner) checkRepo(repo string) error {
	githubURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest(http.MethodGet, githubURL, nil)
	if err != nil {
		return fmt.Errorf("помилка формування запиту: %w", err)
	}

	if s.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.githubToken)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("помилка виконання запиту до GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		log.Printf("[SCANNER] Warning: GitHub API Rate Limit Exceeded для %s\n", repo)
		return fmt.Errorf("rate limit exceeded") // сигналізуємо для переривання
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("отримано неочікуваний статус: %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return fmt.Errorf("помилка парсингу відповіді: %w", err)
	}

	latestTag := rel.TagName
	if latestTag == "" {
		return nil // немає валідного тегу
	}

	lastSeen, err := s.storage.GetLastSeenTag(repo)
	if err != nil {
		log.Printf("[SCANNER] Warning: не вдалось дістати last_seen_tag для %q: %v", repo, err)
	}

	// Якщо тег релізу відрізняється від того, що є в базі, генеруємо подію!
	if latestTag != lastSeen {
		emails, err := s.storage.GetEmailsForRepo(repo)
		if err != nil {
			return fmt.Errorf("помилка отримання підписників: %w", err)
		}

		if len(emails) > 0 {
			if err := s.notifier.SendReleaseNotification(emails, repo, latestTag); err != nil {
				return fmt.Errorf("помилка надсилання сповіщень: %w", err)
			}
		}

		if err := s.storage.UpdateLastSeenTag(repo, latestTag); err != nil {
			return fmt.Errorf("помилка збереження нового тегу: %w", err)
		}
	}

	return nil
}
