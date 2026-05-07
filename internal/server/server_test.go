package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goguson/homework-april-1/internal/config"
	"github.com/goguson/homework-april-1/internal/entities"
	"github.com/goguson/homework-april-1/internal/ingestion"
	"github.com/goguson/homework-april-1/internal/server/api"
)

type fakeRepo struct{}

type fakeRatesFetcher struct {
	currencies []string
	result     ingestion.Result
	err        error
}

func (f *fakeRatesFetcher) FetchAndStore(ctx context.Context, currencies []string) (ingestion.Result, error) {
	f.currencies = append([]string(nil), currencies...)
	return f.result, f.err
}

func (fakeRepo) Ping(ctx context.Context) error { return nil }

func (fakeRepo) LatestRates(ctx context.Context) ([]entities.ExchangeRate, error) {
	return []entities.ExchangeRate{{
		ID:            "019bcb7d-f22c-7bcc-9441-fcd1e5c1ec87",
		BaseCurrency:  "EUR",
		QuoteCurrency: "GBP",
		Rate:          "0.85670000",
		EffectiveDate: time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC),
		Source:        "test",
	}}, nil
}

func (fakeRepo) History(ctx context.Context, currency string, from, to *time.Time, limit int) ([]entities.ExchangeRate, error) {
	return []entities.ExchangeRate{{
		BaseCurrency:  "EUR",
		QuoteCurrency: currency,
		Rate:          "0.85670000",
		EffectiveDate: time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC),
		Source:        "test",
	}}, nil
}

func TestLatestRatesEndpointReturnsJSON(t *testing.T) {
	cfg := config.Config{
		Server:    config.Server{Address: "127.0.0.1:0"},
		RateLimit: config.RateLimit{Enabled: false},
	}
	srv := &Server{cfg: cfg, repo: fakeRepo{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rates", nil)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body api.RatesResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.BaseCurrency != "EUR" || len(body.Rates) != 1 || body.Rates[0].Currency != "GBP" {
		t.Fatalf("unexpected body = %+v", body)
	}
}

func TestDocsEndpointServesEmbeddedOpenAPIUI(t *testing.T) {
	cfg := config.Config{
		Server:    config.Server{Address: "127.0.0.1:0"},
		RateLimit: config.RateLimit{Enabled: false},
	}
	srv := &Server{cfg: cfg, repo: fakeRepo{}}
	req := httptest.NewRequest(http.MethodGet, "/api/docs/", nil)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); contentType == "" || contentType[:9] != "text/html" {
		t.Fatalf("Content-Type = %q", contentType)
	}
}

func TestFetchEndpointUsesGeneratedRouteAndRunsIngestion(t *testing.T) {
	fetcher := &fakeRatesFetcher{
		result: ingestion.Result{
			Saved: []entities.ExchangeRate{{
				BaseCurrency:  "EUR",
				QuoteCurrency: "GBP",
				Rate:          "0.85670000",
				EffectiveDate: time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC),
				Source:        "test",
			}},
		},
	}
	cfg := config.Config{
		Server:    config.Server{Address: "127.0.0.1:0"},
		RateLimit: config.RateLimit{Enabled: false},
	}
	srv := &Server{cfg: cfg, repo: fakeRepo{}, fetcher: fetcher, defaultCurrencies: []string{"USD"}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rates/fetch", bytes.NewBufferString(`{"currencies":["GBP"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if len(fetcher.currencies) != 1 || fetcher.currencies[0] != "GBP" {
		t.Fatalf("currencies = %v", fetcher.currencies)
	}
	var body api.FetchRatesResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Saved) != 1 || body.Saved[0].Currency != "GBP" {
		t.Fatalf("unexpected body = %+v", body)
	}
}

func TestUnknownBrowserPathRedirectsToDocs(t *testing.T) {
	cfg := config.Config{
		Server:    config.Server{Address: "127.0.0.1:0"},
		RateLimit: config.RateLimit{Enabled: false},
	}
	srv := &Server{cfg: cfg, repo: fakeRepo{}}
	req := httptest.NewRequest(http.MethodGet, "/wrong-address", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "/api/docs/" {
		t.Fatalf("Location = %q", location)
	}
}
