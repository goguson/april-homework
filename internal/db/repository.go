package db

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/gofrs/uuid/v5"
	bobmodels "github.com/goguson/homework-april-1/internal/db/bob/bob"
	"github.com/goguson/homework-april-1/internal/db/bob/dberrors"
	"github.com/goguson/homework-april-1/internal/entities"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql/dialect"
	"github.com/stephenafamo/bob/dialect/psql/im"
	"github.com/stephenafamo/bob/dialect/psql/sm"
	bobx "github.com/stephenafamo/bob/drivers/pgx"
)

type Repository struct {
	pool    *pgxpool.Pool
	bobPool bobx.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, bobPool: bobx.NewPool(pool)}
}

func (r *Repository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

func (r *Repository) UpsertRates(ctx context.Context, rates []entities.ExchangeRate) error {
	tx, err := r.bobPool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, rate := range rates {
		setter, err := toExchangeRateSetter(rate)
		if err != nil {
			return err
		}

		_, err = bobmodels.ExchangeRates.Insert(
			setter,
			im.OnConflictOnConstraint(dberrors.ExchangeRateErrors.ErrUniqueExchangeRatesSourceBaseCurrencyQuoteCurrencyEffectivKey.Error()).DoUpdate(
				im.SetExcluded("rate", "source_published_at"),
			),
		).Exec(ctx, tx)
		if err != nil {
			return fmt.Errorf("upsert %s: %w", rate.QuoteCurrency, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *Repository) LatestRates(ctx context.Context) ([]entities.ExchangeRate, error) {
	rows, err := bobmodels.ExchangeRates.Query(
		sm.OrderBy(bobmodels.ExchangeRates.Columns.QuoteCurrency).Asc(),
		sm.OrderBy(bobmodels.ExchangeRates.Columns.EffectiveDate).Desc(),
	).All(ctx, r.bobPool)
	if err != nil {
		return nil, fmt.Errorf("query latest rates: %w", err)
	}

	latest := make(map[string]entities.ExchangeRate)
	for _, row := range rows {
		rate := toEntity(row)
		if _, exists := latest[rate.QuoteCurrency]; !exists {
			latest[rate.QuoteCurrency] = rate
		}
	}

	result := make([]entities.ExchangeRate, 0, len(latest))
	for _, rate := range latest {
		result = append(result, rate)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].QuoteCurrency < result[j].QuoteCurrency
	})
	return result, nil
}

func (r *Repository) History(ctx context.Context, currency string, from, to *time.Time, limit int) ([]entities.ExchangeRate, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	mods := []bob.Mod[*dialect.SelectQuery]{
		bobmodels.SelectWhere.ExchangeRates.QuoteCurrency.EQ(currency),
		sm.OrderBy(bobmodels.ExchangeRates.Columns.EffectiveDate).Desc(),
		sm.Limit(limit),
	}
	if from != nil {
		mods = append(mods, bobmodels.SelectWhere.ExchangeRates.EffectiveDate.GTE(*from))
	}
	if to != nil {
		mods = append(mods, bobmodels.SelectWhere.ExchangeRates.EffectiveDate.LTE(*to))
	}

	rows, err := bobmodels.ExchangeRates.Query(mods...).All(ctx, r.bobPool)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	return toEntities(rows), nil
}

func toExchangeRateSetter(rate entities.ExchangeRate) (*bobmodels.ExchangeRateSetter, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate rate id: %w", err)
	}

	amount, err := decimal.NewFromString(rate.Rate)
	if err != nil {
		return nil, fmt.Errorf("parse %s rate %q: %w", rate.QuoteCurrency, rate.Rate, err)
	}

	setter := &bobmodels.ExchangeRateSetter{
		ID:            omit.From(id),
		BaseCurrency:  omit.From(rate.BaseCurrency),
		QuoteCurrency: omit.From(rate.QuoteCurrency),
		Rate:          omit.From(amount),
		EffectiveDate: omit.From(rate.EffectiveDate),
		Source:        omit.From(rate.Source),
	}
	if !rate.SourcePublishedAt.IsZero() {
		setter.SourcePublishedAt = omitnull.From(rate.SourcePublishedAt)
	}
	return setter, nil
}

func toEntities(rows bobmodels.ExchangeRateSlice) []entities.ExchangeRate {
	result := make([]entities.ExchangeRate, len(rows))
	for i, row := range rows {
		result[i] = toEntity(row)
	}
	return result
}

func toEntity(row *bobmodels.ExchangeRate) entities.ExchangeRate {
	publishedAt := time.Time{}
	if !row.SourcePublishedAt.IsNull() {
		publishedAt = row.SourcePublishedAt.MustGet()
	}

	return entities.ExchangeRate{
		ID:                row.ID.String(),
		BaseCurrency:      row.BaseCurrency,
		QuoteCurrency:     row.QuoteCurrency,
		Rate:              row.Rate.StringFixed(8),
		EffectiveDate:     row.EffectiveDate,
		Source:            row.Source,
		SourcePublishedAt: publishedAt,
	}
}
