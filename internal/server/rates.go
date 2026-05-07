package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/goguson/homework-april-1/internal/entities"
	"github.com/goguson/homework-april-1/internal/server/api"
)

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
