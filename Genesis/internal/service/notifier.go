package service

import (
	"fmt"
	"log"
	"net/smtp"
)

// Notifier визначає загальний інтерфейс для відправки повідомлень
type Notifier interface {
	SendReleaseNotification(emails []string, repo, releaseTag string) error
}

// LogNotifier є тимчасовою реалізацією, що виводить нотифікації у консоль
type LogNotifier struct{}

// NewLogNotifier створює екземпляр LogNotifier
func NewLogNotifier() Notifier {
	return &LogNotifier{}
}

// SendReleaseNotification друкує повідомлення про відправку в консоль
func (n *LogNotifier) SendReleaseNotification(emails []string, repo, releaseTag string) error {
	log.Printf("[NOTIFIER] Sending release notification for repo %q | New Tag: %q | Recipients (%d): %v\n", repo, releaseTag, len(emails), emails)
	return nil
}

// SMTPNotifier реалізація Notifier для відправки реальних email'ів
type SMTPNotifier struct {
	host     string
	port     string
	user     string
	password string
	from     string
}

// NewSMTPNotifier створює екземпляр SMTPNotifier
func NewSMTPNotifier(host, port, user, password, from string) Notifier {
	return &SMTPNotifier{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		from:     from,
	}
}

// SendReleaseNotification відправляє email через SMTP
func (s *SMTPNotifier) SendReleaseNotification(emails []string, repo, releaseTag string) error {
	if len(emails) == 0 {
		return nil
	}

	auth := smtp.PlainAuth("", s.user, s.password, s.host)
	addr := fmt.Sprintf("%s:%s", s.host, s.port)

	subject := fmt.Sprintf("Subject: New Release for %s!\n", repo)
	mime := "MIME-version: 1.0;\nContent-Type: text/plain; charset=\"UTF-8\";\n\n"
	body := fmt.Sprintf("Hello!\n\nA new release (%s) has been published for the repository %s.\nCheck it out on GitHub!", releaseTag, repo)
	
	msg := []byte(subject + mime + body)

	// Відправка на масив emails. За стандартом до них прийде blind carbon copy (BCC), якщо ми не пишемо заголовок To: в самому тілі `msg`
	err := smtp.SendMail(addr, auth, s.from, emails, msg)
	if err != nil {
		return fmt.Errorf("failed to send emails via SMTP: %w", err)
	}

	log.Printf("[NOTIFIER] Successfully sent SMTP emails to %d recipients for %q", len(emails), repo)
	return nil
}
