package ingestion

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/goguson/homework-april-1/internal/entities"
)

type fakeFetcher struct {
	mu       sync.Mutex
	requests []string
}

func (f *fakeFetcher) FetchCurrency(ctx context.Context, currency string) (entities.ExchangeRate, error) {
	f.mu.Lock()
	f.requests = append(f.requests, currency)
	f.mu.Unlock()

	return entities.ExchangeRate{
		BaseCurrency:  "EUR",
		QuoteCurrency: currency,
		Rate:          "1.23000000",
		EffectiveDate: time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC),
		Source:        "test",
	}, nil
}

type fakeRepository struct {
	mu    sync.Mutex
	rates []entities.ExchangeRate
}

func (f *fakeRepository) UpsertRates(ctx context.Context, rates []entities.ExchangeRate) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rates = append(f.rates, rates...)
	return nil
}

func TestFetchAndStoreFetchesEachCurrencyAndPersistsResults(t *testing.T) {
	fetcher := &fakeFetcher{}
	repo := &fakeRepository{}
	svc := NewService(fetcher, repo, 4)

	result, err := svc.FetchAndStore(context.Background(), []string{"GBP", "USD", "PLN"})
	if err != nil {
		t.Fatalf("FetchAndStore() error = %v", err)
	}
	if len(result.Saved) != 3 {
		t.Fatalf("saved len = %d", len(result.Saved))
	}
	if len(result.Failed) != 0 {
		t.Fatalf("failed len = %d", len(result.Failed))
	}
	if len(repo.rates) != 3 {
		t.Fatalf("repo rates len = %d", len(repo.rates))
	}

	slices.Sort(fetcher.requests)
	want := []string{"GBP", "PLN", "USD"}
	for i := range want {
		if fetcher.requests[i] != want[i] {
			t.Fatalf("requests = %v", fetcher.requests)
		}
	}
}
