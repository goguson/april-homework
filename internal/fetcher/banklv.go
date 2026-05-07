package fetcher

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/goguson/homework-april-1/internal/entities"
)

const BankLVSource = "bank.lv/ecb_rss"

var numberPattern = regexp.MustCompile(`[0-9]+(?:[.,][0-9]+)?`)

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type BankLVFetcher struct {
	url    string
	client HTTPDoer
	logger *slog.Logger
	now    func() time.Time
}

func NewBankLVFetcher(url string, client HTTPDoer, logger *slog.Logger) *BankLVFetcher {
	if url == "" {
		url = "https://www.bank.lv/vk/ecb_rss.xml"
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &BankLVFetcher{
		url:    url,
		client: client,
		logger: logger,
		now:    time.Now,
	}
}

func (f *BankLVFetcher) FetchCurrency(ctx context.Context, currency string) (entities.ExchangeRate, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	f.logger.DebugContext(ctx, "fetch currency started", slog.String("currency", currency), slog.String("url", f.url))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return entities.ExchangeRate{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "rates-service/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return entities.ExchangeRate{}, fmt.Errorf("fetch %s: %w", currency, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return entities.ExchangeRate{}, fmt.Errorf("fetch %s: status %d", currency, resp.StatusCode)
	}

	rate, err := ParseBankLVRSS(io.LimitReader(resp.Body, 2*1024*1024), currency, f.now())
	if err != nil {
		return entities.ExchangeRate{}, err
	}

	f.logger.DebugContext(ctx, "fetch currency completed",
		slog.String("currency", currency),
		slog.String("rate", rate.Rate),
		slog.Time("effective_date", rate.EffectiveDate),
	)
	return rate, nil
}

type rssFeed struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Date        string `xml:"date"`
}

func ParseBankLVRSS(reader io.Reader, currency string, fallbackDate time.Time) (entities.ExchangeRate, error) {
	var feed rssFeed
	if err := xml.NewDecoder(reader).Decode(&feed); err != nil {
		return entities.ExchangeRate{}, fmt.Errorf("decode bank.lv rss: %w", err)
	}

	currency = strings.ToUpper(strings.TrimSpace(currency))
	var latest *entities.ExchangeRate
	for _, item := range feed.Channel.Items {
		text := strings.ToUpper(item.Title + " " + item.Description)
		if !strings.Contains(text, currency) {
			continue
		}

		rateValue, err := extractRate(item.Title+" "+item.Description, currency)
		if err != nil {
			return entities.ExchangeRate{}, fmt.Errorf("parse %s rate: %w", currency, err)
		}

		publishedAt := parseRSSDate(item.PubDate)
		if publishedAt.IsZero() {
			publishedAt = parseRSSDate(item.Date)
		}
		effectiveDate := publishedAt
		if effectiveDate.IsZero() {
			effectiveDate = fallbackDate
		}

		rate := entities.ExchangeRate{
			BaseCurrency:      entities.BaseCurrency,
			QuoteCurrency:     currency,
			Rate:              rateValue,
			EffectiveDate:     dateOnly(effectiveDate),
			Source:            BankLVSource,
			SourcePublishedAt: publishedAt,
		}
		if latest == nil || rate.EffectiveDate.After(latest.EffectiveDate) {
			latest = &rate
		}
	}

	if latest != nil {
		return *latest, nil
	}

	return entities.ExchangeRate{}, fmt.Errorf("currency %s not found", currency)
}

func extractRate(text, currency string) (string, error) {
	text = strings.ReplaceAll(text, ",", ".")
	fields := strings.Fields(text)
	for i, field := range fields {
		field = cleanToken(field)
		if strings.EqualFold(field, currency) {
			if i+1 < len(fields) {
				return normalizeRate(fields[i+1], currency)
			}
			if i > 0 {
				return normalizeRate(fields[i-1], currency)
			}
		}
	}

	return "", fmt.Errorf("no numeric rate for %s", currency)
}

func normalizeRate(value string, currency string) (string, error) {
	value = cleanToken(value)
	if !numberPattern.MatchString(value) {
		return "", fmt.Errorf("no numeric rate for %s", currency)
	}

	rat, ok := new(big.Rat).SetString(value)
	if !ok || rat.Sign() <= 0 {
		return "", fmt.Errorf("invalid rate %q for %s", value, currency)
	}
	return rat.FloatString(8), nil
}

func cleanToken(value string) string {
	return strings.Trim(value, " \t\r\n:;,.()[]{}")
}

func parseRSSDate(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "2006-01-02"}
	for _, layout := range layouts {
		t, err := time.Parse(layout, value)
		if err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
