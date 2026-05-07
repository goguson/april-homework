package entities

import "time"

const BaseCurrency = "EUR"

var DefaultCurrencies = []string{"USD", "GBP", "JPY", "CHF", "PLN", "SEK", "NOK", "DKK", "CAD", "AUD"}

type ExchangeRate struct {
	ID                string
	BaseCurrency      string
	QuoteCurrency     string
	Rate              string
	EffectiveDate     time.Time
	Source            string
	SourcePublishedAt time.Time
}

type FetchFailure struct {
	Currency string `json:"currency"`
	Error    string `json:"error"`
}
