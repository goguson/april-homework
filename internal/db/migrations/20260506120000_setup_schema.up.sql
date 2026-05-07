CREATE SCHEMA IF NOT EXISTS rates;

SET search_path TO rates;

CREATE TABLE IF NOT EXISTS exchange_rates (
    id UUID PRIMARY KEY NOT NULL,
    base_currency CHAR(3) NOT NULL DEFAULT 'EUR',
    quote_currency CHAR(3) NOT NULL,
    rate NUMERIC(20, 8) NOT NULL,
    effective_date DATE NOT NULL,
    source TEXT NOT NULL,
    source_published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (source, base_currency, quote_currency, effective_date)
);

CREATE INDEX IF NOT EXISTS exchange_rates_quote_currency_effective_date_idx
    ON exchange_rates (quote_currency, effective_date DESC);
