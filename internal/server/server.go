package server

import (
	"context"
	"encoding/json"
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
