package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

var (
	AppName    = "rates-service"
	Version    = "v0.0.0-local"
	CommitHash string
	BuildDate  string
)

type Config struct {
	App       App
	Server    Server
	Database  Database
	Redis     Redis
	Fetcher   Fetcher
	RateLimit RateLimit
}

type App struct {
	LogLevel string
}

type Server struct {
	Host         string
	Port         int
	Address      string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type Database struct {
	URI string
}

type Redis struct {
	Address  string
	Password string
	DB       int
}

type Fetcher struct {
	BankLVURL   string
	Currencies  []string
	Concurrency int
}

type RateLimit struct {
	Enabled bool
	Limit   int
	Window  time.Duration
}

type Input struct {
	DBUsername string `env:"DB_USERNAME" envDefault:"local_user"`
	DBPassword string `env:"DB_PASSWORD" envDefault:"local_db"`
	DBHost     string `env:"DB_HOST" envDefault:"localhost"`
	DBPort     string `env:"DB_PORT" envDefault:"5432"`
	DBName     string `env:"DB_NAME" envDefault:"rates-service"`
	DBSchema   string `env:"DB_SCHEMA" envDefault:"rates"`

	RedisAddress  string `env:"REDIS_ADDRESS" envDefault:"localhost:6379"`
	RedisPassword string `env:"REDIS_PASSWORD" envDefault:""`
	RedisDB       int    `env:"REDIS_DB" envDefault:"0"`

	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	Host         string `env:"HOST" envDefault:"0.0.0.0"`
	Port         int    `env:"PORT" envDefault:"8888"`
	ReadTimeout  int    `env:"READ_TIMEOUT" envDefault:"5"`
	WriteTimeout int    `env:"WRITE_TIMEOUT" envDefault:"5"`

	BankLVURL          string `env:"BANK_LV_URL" envDefault:"https://www.bank.lv/vk/ecb_rss.xml"`
	SelectedCurrencies string `env:"SELECTED_CURRENCIES" envDefault:"USD,GBP,JPY,CHF,PLN,SEK,NOK,DKK,CAD,AUD"`
	FetchConcurrency   int    `env:"FETCH_CONCURRENCY" envDefault:"4"`

	RateLimitEnabled bool `env:"RATE_LIMIT_ENABLED" envDefault:"true"`
	RateLimitLimit   int  `env:"RATE_LIMIT_LIMIT" envDefault:"120"`
	RateLimitWindow  int  `env:"RATE_LIMIT_WINDOW_SECONDS" envDefault:"60"`
}

func (i Input) Load() (Config, error) {
	if i.Port < 1 || i.Port > 65535 {
		return Config{}, fmt.Errorf("expected port range from 1 to 65535, received: %d", i.Port)
	}
	if i.FetchConcurrency < 1 {
		return Config{}, fmt.Errorf("FETCH_CONCURRENCY must be greater than zero")
	}

	q := url.Values{}
	q.Set("sslmode", "disable")
	if i.DBSchema != "" {
		q.Set("search_path", i.DBSchema)
	}

	userInfo := url.UserPassword(i.DBUsername, i.DBPassword)
	dbURI := fmt.Sprintf("postgres://%s@%s:%s/%s?%s", userInfo.String(), i.DBHost, i.DBPort, i.DBName, q.Encode())

	return Config{
		App: App{LogLevel: i.LogLevel},
		Server: Server{
			Host:         i.Host,
			Port:         i.Port,
			Address:      fmt.Sprintf("%s:%d", i.Host, i.Port),
			ReadTimeout:  time.Duration(i.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(i.WriteTimeout) * time.Second,
		},
		Database: Database{URI: dbURI},
		Redis: Redis{
			Address:  i.RedisAddress,
			Password: i.RedisPassword,
			DB:       i.RedisDB,
		},
		Fetcher: Fetcher{
			BankLVURL:   i.BankLVURL,
			Currencies:  splitCurrencies(i.SelectedCurrencies),
			Concurrency: i.FetchConcurrency,
		},
		RateLimit: RateLimit{
			Enabled: i.RateLimitEnabled,
			Limit:   i.RateLimitLimit,
			Window:  time.Duration(i.RateLimitWindow) * time.Second,
		},
	}, nil
}

func splitCurrencies(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ToUpper(strings.TrimSpace(part))
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
