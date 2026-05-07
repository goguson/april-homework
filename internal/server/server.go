package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/goguson/homework-april-1/docs"
	"github.com/goguson/homework-april-1/internal/config"
	"github.com/goguson/homework-april-1/internal/entities"
	"github.com/goguson/homework-april-1/internal/ingestion"
	"github.com/goguson/homework-april-1/internal/server/api"
	openapiTypes "github.com/oapi-codegen/runtime/types"
	"github.com/redis/go-redis/v9"
)

type Repository interface {
	Ping(ctx context.Context) error
	LatestRates(ctx context.Context) ([]entities.ExchangeRate, error)
	History(ctx context.Context, currency string, from, to *time.Time, limit int) ([]entities.ExchangeRate, error)
}

type RatesFetcher interface {
	FetchAndStore(ctx context.Context, currencies []string) (ingestion.Result, error)
}

type Server struct {
	cfg               config.Config
	repo              Repository
	fetcher           RatesFetcher
	defaultCurrencies []string
	redis             *redis.Client
	logger            *slog.Logger
}

var _ api.StrictServerInterface = (*Server)(nil)

func New(cfg config.Config, repo Repository, fetcher RatesFetcher, redisClient *redis.Client, logger *slog.Logger) *http.Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		cfg:               cfg,
		repo:              repo,
		fetcher:           fetcher,
		defaultCurrencies: cfg.Fetcher.Currencies,
		redis:             redisClient,
		logger:            logger,
	}
	return &http.Server{
		Addr:         cfg.Server.Address,
		Handler:      s.routes(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		BaseContext: func(net.Listener) context.Context {
			return context.Background()
		},
	}
}

func (s *Server) routes() http.Handler {
	if s.logger == nil {
		s.logger = slog.Default()
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(s.logMiddleware)
	if s.cfg.RateLimit.Enabled && s.redis != nil {
		r.Use(s.rateLimitMiddleware)
	}

	r.Get("/health", s.health)
	r.Get("/", redirectToDocs)
	api.HandlerFromMuxWithBaseURL(
		api.NewStrictHandlerWithOptions(
			s,
			nil,
			api.StrictHTTPServerOptions{
				RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
					writeError(w, http.StatusUnprocessableEntity, err.Error())
				},
				ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
					s.logger.ErrorContext(r.Context(), "api response failed", slog.String("error", err.Error()))
					writeError(w, http.StatusInternalServerError, "internal error")
				},
			},
		),
		r,
		"/api/v1",
	)
	docs.NewOpenAPI(r)
	r.NotFound(notFound)
	return r
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.repo.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func notFound(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && wantsHTML(r) {
		redirectToDocs(w, r)
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}

func redirectToDocs(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/api/docs/", http.StatusFound)
}

func wantsHTML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

func (s *Server) GetRates(ctx context.Context, request api.GetRatesRequestObject) (api.GetRatesResponseObject, error) {
	rates, err := s.repo.LatestRates(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "latest rates failed", slog.String("error", err.Error()))
		return nil, fmt.Errorf("load latest rates: %w", err)
	}
	return api.GetRates200JSONResponse{
		BaseCurrency: entities.BaseCurrency,
		Rates:        toResponses(rates),
	}, nil
}

func (s *Server) PostRatesFetch(ctx context.Context, request api.PostRatesFetchRequestObject) (api.PostRatesFetchResponseObject, error) {
	if s.fetcher == nil {
		return api.PostRatesFetch500JSONResponse{Error: "fetcher is not configured"}, nil
	}

	currencies := s.defaultCurrencies
	if request.Body != nil && request.Body.Currencies != nil && len(*request.Body.Currencies) > 0 {
		currencies = *request.Body.Currencies
	}
	if len(currencies) == 0 {
		currencies = entities.DefaultCurrencies
	}

	result, err := s.fetcher.FetchAndStore(ctx, currencies)
	response := api.FetchRatesResponse{
		Saved:  toResponses(result.Saved),
		Failed: toFetchFailures(result.Failed),
	}
	if err != nil && len(result.Saved) == 0 {
		return api.PostRatesFetch500JSONResponse{Error: err.Error()}, nil
	}
	if err != nil || len(result.Failed) > 0 {
		return api.PostRatesFetch207JSONResponse(response), nil
	}
	return api.PostRatesFetch200JSONResponse(response), nil
}

func (s *Server) GetRatesCurrencyHistory(ctx context.Context, request api.GetRatesCurrencyHistoryRequestObject) (api.GetRatesCurrencyHistoryResponseObject, error) {
	currency := strings.ToUpper(strings.TrimSpace(request.Currency))
	if len(currency) != 3 {
		return api.GetRatesCurrencyHistory422JSONResponse{Error: "currency must be ISO 4217 code"}, nil
	}

	limit := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}

	rates, err := s.repo.History(
		ctx,
		currency,
		dateParamToTime(request.Params.From),
		dateParamToTime(request.Params.To),
		limit,
	)
	if err != nil {
		s.logger.ErrorContext(ctx, "history failed", slog.String("currency", currency), slog.String("error", err.Error()))
		return nil, fmt.Errorf("load rate history: %w", err)
	}
	return api.GetRatesCurrencyHistory200JSONResponse{
		BaseCurrency: entities.BaseCurrency,
		Rates:        toResponses(rates),
	}, nil
}

func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := fmt.Sprintf("rate_limit:%s:%s", r.URL.Path, clientIP(r))
		count, err := s.redis.Incr(r.Context(), key).Result()
		if err != nil {
			s.logger.ErrorContext(r.Context(), "redis rate limit failed", slog.String("error", err.Error()))
			next.ServeHTTP(w, r)
			return
		}
		if count == 1 {
			_ = s.redis.Expire(r.Context(), key, s.cfg.RateLimit.Window).Err()
		}
		if count > int64(s.cfg.RateLimit.Limit) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		s.logger.DebugContext(r.Context(), "request completed",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Duration("duration", time.Since(started)),
		)
	})
}

func toResponses(rates []entities.ExchangeRate) []api.Rate {
	result := make([]api.Rate, len(rates))
	for i, rate := range rates {
		id := rate.ID
		result[i] = api.Rate{
			Currency:      rate.QuoteCurrency,
			Rate:          rate.Rate,
			EffectiveDate: openapiTypes.Date{Time: rate.EffectiveDate},
			Source:        rate.Source,
		}
		if id != "" {
			result[i].Id = &id
		}
	}
	return result
}

func dateParamToTime(value *openapiTypes.Date) *time.Time {
	if value == nil {
		return nil
	}
	t := value.Time
	return &t
}

func toFetchFailures(failures []entities.FetchFailure) []api.FetchFailure {
	result := make([]api.FetchFailure, len(failures))
	for i, failure := range failures {
		result[i] = api.FetchFailure{
			Currency: failure.Currency,
			Error:    failure.Error,
		}
	}
	return result
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
