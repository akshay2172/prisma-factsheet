-- ============================================================
-- PRISMA FACTSHEET — Database Schema
-- PostgreSQL 15+
-- ============================================================

-- Enable extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- ============================================================
-- PRODUCTS & PORTFOLIOS
-- ============================================================

CREATE TABLE products (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    slug          TEXT UNIQUE NOT NULL,          -- e.g. "global-growth-prisma"
    name          TEXT NOT NULL,                 -- e.g. "Global Growth Prisma"
    description   TEXT,
    product_type  TEXT NOT NULL DEFAULT 'prisma', -- prisma | etf | fund
    currency      TEXT NOT NULL DEFAULT 'USD',
    inception_date DATE,
    benchmark_id  UUID,                          -- FK to benchmarks (set after)
    is_active     BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE benchmarks (
    id        UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    ticker    TEXT UNIQUE NOT NULL,
    name      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE products
    ADD CONSTRAINT fk_products_benchmark
    FOREIGN KEY (benchmark_id) REFERENCES benchmarks(id);

-- ============================================================
-- HOLDINGS
-- ============================================================

CREATE TABLE securities (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    ticker       TEXT NOT NULL,
    exchange     TEXT,                           -- NASDAQ, NYSE, LSE ...
    isin         TEXT,
    name         TEXT NOT NULL,
    asset_class  TEXT NOT NULL,                  -- equity | etf | bond | cash
    country_code TEXT,                           -- ISO 3166-1 alpha-2
    sector       TEXT,                           -- GICS sector
    industry     TEXT,
    currency     TEXT NOT NULL DEFAULT 'USD',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (ticker, exchange)
);

CREATE INDEX idx_securities_ticker ON securities(ticker);
CREATE INDEX idx_securities_country ON securities(country_code);
CREATE INDEX idx_securities_sector  ON securities(sector);

-- Point-in-time holdings (each rebalance stored as a snapshot)
CREATE TABLE holdings (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    product_id    UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    security_id   UUID NOT NULL REFERENCES securities(id),
    as_of_date    DATE NOT NULL,
    weight        NUMERIC(8, 6) NOT NULL,        -- 0.0 – 1.0
    quantity      NUMERIC(18, 6),
    market_value  NUMERIC(18, 4),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (product_id, security_id, as_of_date)
);

CREATE INDEX idx_holdings_product_date ON holdings(product_id, as_of_date DESC);

-- ============================================================
-- MARKET DATA
-- ============================================================

CREATE TABLE price_history (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    security_id   UUID NOT NULL REFERENCES securities(id) ON DELETE CASCADE,
    date          DATE NOT NULL,
    open          NUMERIC(18, 6),
    high          NUMERIC(18, 6),
    low           NUMERIC(18, 6),
    close         NUMERIC(18, 6) NOT NULL,
    adj_close     NUMERIC(18, 6) NOT NULL,
    volume        BIGINT,
    source        TEXT NOT NULL DEFAULT 'yahoo_finance',
    fetched_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (security_id, date)
);

CREATE INDEX idx_price_history_security_date ON price_history(security_id, date DESC);

-- ============================================================
-- NAV / PERFORMANCE
-- ============================================================

CREATE TABLE nav_history (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    product_id    UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    date          DATE NOT NULL,
    nav           NUMERIC(18, 6) NOT NULL,
    daily_return  NUMERIC(12, 8),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (product_id, date)
);

CREATE INDEX idx_nav_history_product_date ON nav_history(product_id, date DESC);

-- Pre-computed performance metrics (refreshed daily by ETL)
CREATE TABLE performance_metrics (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    product_id        UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    as_of_date        DATE NOT NULL,
    -- Returns
    return_1d         NUMERIC(10, 6),
    return_1w         NUMERIC(10, 6),
    return_1m         NUMERIC(10, 6),
    return_3m         NUMERIC(10, 6),
    return_6m         NUMERIC(10, 6),
    return_ytd        NUMERIC(10, 6),
    return_1y         NUMERIC(10, 6),
    return_3y         NUMERIC(10, 6),
    return_inception  NUMERIC(10, 6),
    -- Risk
    volatility_1y     NUMERIC(10, 6),
    sharpe_ratio_1y   NUMERIC(10, 6),
    max_drawdown_1y   NUMERIC(10, 6),
    beta_1y           NUMERIC(10, 6),
    -- vs Benchmark
    benchmark_return_1y  NUMERIC(10, 6),
    alpha_1y             NUMERIC(10, 6),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (product_id, as_of_date)
);

-- ============================================================
-- EXPOSURE BREAKDOWNS (country / sector / market-cap)
-- ============================================================

CREATE TABLE exposure_snapshots (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    product_id    UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    as_of_date    DATE NOT NULL,
    dimension     TEXT NOT NULL,   -- 'country' | 'sector' | 'market_cap_tier'
    label         TEXT NOT NULL,   -- e.g. "United States", "Technology", "Large Cap"
    weight        NUMERIC(8, 6) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_exposure_product_date ON exposure_snapshots(product_id, as_of_date DESC, dimension);

-- ============================================================
-- ETL / PIPELINE AUDIT
-- ============================================================

CREATE TABLE etl_runs (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pipeline      TEXT NOT NULL,   -- 'price_fetch' | 'nav_calc' | 'exposure_calc' | 'perf_calc'
    status        TEXT NOT NULL,   -- 'running' | 'success' | 'failed'
    started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at   TIMESTAMPTZ,
    records_processed INT DEFAULT 0,
    error_message TEXT,
    metadata      JSONB
);

CREATE INDEX idx_etl_runs_pipeline ON etl_runs(pipeline, started_at DESC);

-- ============================================================
-- SEED DATA
-- ============================================================

INSERT INTO benchmarks (id, ticker, name)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'ACWI',  'MSCI All Country World Index'),
    ('00000000-0000-0000-0000-000000000002', 'SPY',   'S&P 500 ETF'),
    ('00000000-0000-0000-0000-000000000003', 'VT',    'Vanguard Total World Stock ETF');

INSERT INTO products (id, slug, name, description, inception_date, benchmark_id)
VALUES (
    '10000000-0000-0000-0000-000000000001',
    'global-growth-prisma',
    'Global Growth Prisma',
    'A globally diversified equity portfolio targeting long-term capital appreciation through exposure to high-quality growth companies across developed and emerging markets.',
    '2023-01-01',
    '00000000-0000-0000-0000-000000000001'
);

INSERT INTO securities (ticker, exchange, name, asset_class, country_code, sector, currency) VALUES
    ('AAPL',  'NASDAQ', 'Apple Inc.',                   'equity', 'US', 'Information Technology', 'USD'),
    ('MSFT',  'NASDAQ', 'Microsoft Corporation',        'equity', 'US', 'Information Technology', 'USD'),
    ('AMZN',  'NASDAQ', 'Amazon.com Inc.',              'equity', 'US', 'Consumer Discretionary',  'USD'),
    ('GOOGL', 'NASDAQ', 'Alphabet Inc.',                'equity', 'US', 'Communication Services',  'USD'),
    ('NVDA',  'NASDAQ', 'NVIDIA Corporation',           'equity', 'US', 'Information Technology', 'USD'),
    ('META',  'NASDAQ', 'Meta Platforms Inc.',          'equity', 'US', 'Communication Services',  'USD'),
    ('TSM',   'NYSE',   'Taiwan Semiconductor Mfg',     'equity', 'TW', 'Information Technology', 'USD'),
    ('ASML',  'NASDAQ', 'ASML Holding NV',              'equity', 'NL', 'Information Technology', 'USD'),
    ('NOVO-B','CPH',    'Novo Nordisk A/S',             'equity', 'DK', 'Health Care',             'DKK'),
    ('7203',  'TYO',    'Toyota Motor Corporation',     'equity', 'JP', 'Consumer Discretionary',  'JPY'),
    ('005930','KRX',    'Samsung Electronics',          'equity', 'KR', 'Information Technology', 'KRW'),
    ('BABA',  'NYSE',   'Alibaba Group Holding',        'equity', 'CN', 'Consumer Discretionary',  'USD');