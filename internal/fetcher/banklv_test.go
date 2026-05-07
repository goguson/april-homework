package fetcher

import (
	"strings"
	"testing"
	"time"
)

func TestParseBankLVRSSFindsRequestedCurrency(t *testing.T) {
	rss := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>USD/EUR</title>
      <description>USD 1.0867</description>
      <pubDate>Tue, 05 May 2026 15:00:00 +0000</pubDate>
    </item>
    <item>
      <title>GBP/EUR</title>
      <description>1 EUR = 0.856700 GBP</description>
      <pubDate>Tue, 05 May 2026 15:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

	rate, err := ParseBankLVRSS(strings.NewReader(rss), "GBP", time.Date(2026, 5, 6, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ParseBankLVRSS() error = %v", err)
	}
	if rate.BaseCurrency != "EUR" {
		t.Fatalf("BaseCurrency = %q", rate.BaseCurrency)
	}
	if rate.QuoteCurrency != "GBP" {
		t.Fatalf("QuoteCurrency = %q", rate.QuoteCurrency)
	}
	if rate.Rate != "0.85670000" {
		t.Fatalf("Rate = %q", rate.Rate)
	}
	wantDate := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	if !rate.EffectiveDate.Equal(wantDate) {
		t.Fatalf("EffectiveDate = %s", rate.EffectiveDate)
	}
	if rate.Source != "bank.lv/ecb_rss" {
		t.Fatalf("Source = %q", rate.Source)
	}
}

func TestParseBankLVRSSReturnsLatestRequestedCurrencyRate(t *testing.T) {
	rss := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>ECB EXCHANGE RATES.</title>
      <description>AUD 1.62890000 GBP 0.86358000 USD 1.17000000</description>
      <pubDate>Mon, 04 May 2026 03:00:00 +0300</pubDate>
    </item>
    <item>
      <title>ECB EXCHANGE RATES.</title>
      <description>AUD 1.62990000 GBP 0.86343000 USD 1.16860000</description>
      <pubDate>Tue, 05 May 2026 03:00:00 +0300</pubDate>
    </item>
  </channel>
</rss>`

	rate, err := ParseBankLVRSS(strings.NewReader(rss), "GBP", time.Date(2026, 5, 6, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ParseBankLVRSS() error = %v", err)
	}
	if rate.Rate != "0.86343000" {
		t.Fatalf("Rate = %q", rate.Rate)
	}
	wantDate := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	if !rate.EffectiveDate.Equal(wantDate) {
		t.Fatalf("EffectiveDate = %s", rate.EffectiveDate)
	}
}

func TestParseBankLVRSSReturnsErrorWhenCurrencyMissing(t *testing.T) {
	rss := `<rss version="2.0"><channel><item><title>USD/EUR</title><description>USD 1.0867</description></item></channel></rss>`

	_, err := ParseBankLVRSS(strings.NewReader(rss), "GBP", time.Now())
	if err == nil || !strings.Contains(err.Error(), "currency GBP not found") {
		t.Fatalf("ParseBankLVRSS() error = %v", err)
	}
}
