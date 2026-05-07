package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/goguson/homework-april-1/internal/config"
	"github.com/goguson/homework-april-1/internal/db"
	"github.com/goguson/homework-april-1/internal/entities"
	"github.com/goguson/homework-april-1/internal/fetcher"
	"github.com/goguson/homework-april-1/internal/ingestion"
	"github.com/goguson/homework-april-1/internal/server"
	"github.com/goguson/homework-april-1/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cfg := loadConfig()
	logger := newLogger(cfg.App.LogLevel)
	slog.SetDefault(logger)

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "serve":
		if err := runServe(ctx, cfg, logger); err != nil {
			log.Fatal(err)
		}
	case "fetch":
		if err := runFetch(ctx, cfg, logger); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\nusage: rates-service [serve|fetch]\n", cmd)
		os.Exit(2)
	}
}

func runServe(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pool, err := pgxpool.New(ctx, cfg.Database.URI)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	repo := db.NewRepository(pool)
	bankLV := fetcher.NewBankLVFetcher(cfg.Fetcher.BankLVURL, &http.Client{}, logger)
	ingestionSvc := ingestion.NewService(bankLV, repo, cfg.Fetcher.Concurrency).WithLogger(logger)
	httpServer := server.New(cfg, repo, ingestionSvc, redisClient, logger)
	logger.InfoContext(ctx, "starting http server",
		slog.String("address", cfg.Server.Address),
		slog.String("docs_url", docsURL(cfg.Server.Host, cfg.Server.Port)),
	)
	return service.New(httpServer).Run(ctx)
}

func runFetch(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pool, err := pgxpool.New(ctx, cfg.Database.URI)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	repo := db.NewRepository(pool)
	httpClient := &http.Client{}
	bankLV := fetcher.NewBankLVFetcher(cfg.Fetcher.BankLVURL, httpClient, logger)
	svc := ingestion.NewService(bankLV, repo, cfg.Fetcher.Concurrency).WithLogger(logger)

	currencies := cfg.Fetcher.Currencies
	if len(currencies) == 0 {
		currencies = entities.DefaultCurrencies
	}

	result, err := svc.FetchAndStore(ctx, currencies)
	data, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr == nil {
		fmt.Println(string(data))
	}
	if err != nil {
		return err
	}
	return nil
}

func loadConfig() config.Config {
	input, err := env.ParseAs[config.Input]()
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := input.Load()
	if err != nil {
		log.Fatal(err)
	}
	return cfg
}

func newLogger(level string) *slog.Logger {
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel}))
}

func docsURL(host string, port int) string {
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%d/api/docs/", host, port)
}
