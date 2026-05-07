package ingestion

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/goguson/homework-april-1/internal/entities"
	"golang.org/x/sync/errgroup"
)

type CurrencyRateFetcher interface {
	FetchCurrency(ctx context.Context, currency string) (entities.ExchangeRate, error)
}

type Repository interface {
	UpsertRates(ctx context.Context, rates []entities.ExchangeRate) error
}

type Service struct {
	fetcher     CurrencyRateFetcher
	repository  Repository
	concurrency int
	logger      *slog.Logger
}

type Result struct {
	Saved  []entities.ExchangeRate `json:"saved"`
	Failed []entities.FetchFailure `json:"failed"`
}

type fetchOutcome struct {
	rate    entities.ExchangeRate
	failure entities.FetchFailure
}

func NewService(fetcher CurrencyRateFetcher, repository Repository, concurrency int) *Service {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Service{
		fetcher:     fetcher,
		repository:  repository,
		concurrency: concurrency,
		logger:      slog.Default(),
	}
}

func (s *Service) WithLogger(logger *slog.Logger) *Service {
	if logger != nil {
		s.logger = logger
	}
	return s
}

func (s *Service) FetchAndStore(ctx context.Context, currencies []string) (Result, error) {
	currencies = normalizeCurrencies(currencies)
	if len(currencies) == 0 {
		return Result{}, errors.New("no currencies selected")
	}

	result := Result{
		Saved:  make([]entities.ExchangeRate, 0, len(currencies)),
		Failed: make([]entities.FetchFailure, 0, len(currencies)),
	}
	outcomes := make(chan fetchOutcome, len(currencies))

	group, gCtx := errgroup.WithContext(ctx)
	group.SetLimit(s.concurrency)

	for _, currency := range currencies {
		currency := currency
		group.Go(func() error {
			rate, err := s.fetcher.FetchCurrency(gCtx, currency)
			if err != nil {
				s.logger.ErrorContext(gCtx, "fetch currency failed", slog.String("currency", currency), slog.String("error", err.Error()))
				outcomes <- fetchOutcome{failure: entities.FetchFailure{Currency: currency, Error: err.Error()}}
				return nil
			}
			outcomes <- fetchOutcome{rate: rate}
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		close(outcomes)
		return result, err
	}
	close(outcomes)

	for outcome := range outcomes {
		if outcome.failure.Currency != "" {
			result.Failed = append(result.Failed, outcome.failure)
			continue
		}
		result.Saved = append(result.Saved, outcome.rate)
	}

	if len(result.Saved) > 0 {
		if err := s.repository.UpsertRates(ctx, result.Saved); err != nil {
			return result, fmt.Errorf("upsert rates: %w", err)
		}
	}

	if len(result.Failed) > 0 {
		return result, fmt.Errorf("failed to fetch %d/%d currencies", len(result.Failed), len(currencies))
	}

	return result, nil
}

func normalizeCurrencies(currencies []string) []string {
	seen := make(map[string]struct{}, len(currencies))
	result := make([]string, 0, len(currencies))
	for _, currency := range currencies {
		currency = strings.ToUpper(strings.TrimSpace(currency))
		if currency == "" {
			continue
		}
		if _, ok := seen[currency]; ok {
			continue
		}
		seen[currency] = struct{}{}
		result = append(result, currency)
	}
	return result
}
