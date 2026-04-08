package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"genesis/internal/handlers"
	"genesis/internal/service"
	"genesis/internal/storage"
)

func main() {
	// 1. Завантаження .env файлу
	if err := godotenv.Load(); err != nil {
		log.Println("Не вдалося завантажити файл .env. Використовуються системні змінні оточення.")
	}

	// 2. Читання змінних для підключення до БД
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")

	// Встановлюємо значення за замовчуванням, якщо .env не прочитався
	if dbHost == "" {
		dbHost = "localhost"
	}
	if dbPort == "" {
		dbPort = "5433" // Використовуємо наш новий порт 5433
	}

	appPort := os.Getenv("APP_PORT")
	if appPort == "" {
		appPort = "8080"
	}
	
	githubToken := os.Getenv("GITHUB_TOKEN")

	// 3. Формування DSN (Data Source Name)
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		dbUser, dbPassword, dbHost, dbPort, dbName,
	)
	log.Println("Спроба підключення з DSN:", dsn)

	// 4. Підключення до бази даних
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Помилка підключення до PostgreSQL: %v", err)
	}
	defer db.Close()

	// Перевірка з'єднання
	if err := db.Ping(); err != nil {
		log.Fatalf("База даних недоступна (ping failed): %v", err)
	}
	log.Println("Успішне підключення до PostgreSQL!")

	// 5. Запуск міграцій бази даних
	log.Println("Запуск міграцій бази даних...")
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatalf("Помилка створення драйвера БД для міграцій: %v", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://migrations",
		"postgres", driver)
	if err != nil {
		log.Fatalf("Помилка ініціалізації міграцій: %v", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("Помилка застосування міграцій: %v", err)
	}
	log.Println("Міграції успішно застосовані!")

	// 6. Ініціалізація Storage, Handlers та Сервісів
	st := storage.NewPostgresStorage(db)
	h := handlers.NewHandler(st)
	
	// Ініціалізація нотифікатора (SMTP або фоллбек на Log)
	var notifier service.Notifier
	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost != "" {
		smtpPort := os.Getenv("SMTP_PORT")
		smtpUser := os.Getenv("SMTP_USER")
		smtpPass := os.Getenv("SMTP_PASSWORD")
		smtpFrom := os.Getenv("SMTP_FROM")
		if smtpFrom == "" {
			smtpFrom = "noreply@genesis.local"
		}
		notifier = service.NewSMTPNotifier(smtpHost, smtpPort, smtpUser, smtpPass, smtpFrom)
		log.Println("SMTP Notifier ініціалізовано.")
	} else {
		notifier = service.NewLogNotifier()
		log.Println("SMTP_HOST не задано. Використовується LogNotifier.")
	}

	scannerInterval := 5 * time.Minute // можна теж винести в .env
	scanner := service.NewScanner(st, notifier, scannerInterval, githubToken)
	
	// Запуск фонового воркера
	scanner.Start()

	// 7. Ініціалізація роутера Chi
	r := chi.NewRouter()

	// Додаємо базові middleware (логіювання, відновлення після паніки)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Роути API
	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})
	
	r.Post("/subscribe", h.Subscribe)
	r.Delete("/subscribe/{id}", h.Unsubscribe)

	// 8. Запуск сервера
	serverAddr := fmt.Sprintf(":%s", appPort)
	log.Printf("Сервер запущено. Порт: %s\n", appPort)

	if err := http.ListenAndServe(serverAddr, r); err != nil {
		log.Fatalf("Помилка запуску HTTP сервера: %v", err)
	}
}
