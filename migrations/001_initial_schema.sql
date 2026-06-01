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
    ('7203.T',  'TYO',    'Toyota Motor Corporation',     'equity', 'JP', 'Consumer Discretionary',  'JPY'),
    ('005930.KS','KRX',    'Samsung Electronics',          'equity', 'KR', 'Information Technology', 'KRW'),
    ('BABA',  'NYSE',   'Alibaba Group Holding',        'equity', 'CN', 'Consumer Discretionary',  'USD');

-- ------------------------------------------------------------
-- HOLDINGS (seed for ETL: NAV / exposures / performance)
-- ------------------------------------------------------------

-- Single holdings snapshot. ETL uses MAX(as_of_date) <= requested date.
INSERT INTO holdings (product_id, security_id, as_of_date, weight, quantity, market_value)
SELECT
    '10000000-0000-0000-0000-000000000001'::uuid                           AS product_id,
    s.id                                                                       AS security_id,
    DATE '2024-01-15'                                                         AS as_of_date,
    v.weight                                                                   AS weight,
    v.quantity                                                                 AS quantity,
    v.market_value                                                             AS market_value
FROM securities s
JOIN (
    VALUES
        ('AAPL',   0.095,  120.000000,  180000.00),
        ('MSFT',   0.090,  100.000000,  190000.00),
        ('AMZN',   0.075,  250.000000,  170000.00),
        ('GOOGL',  0.085,  140.000000,  175000.00),
        ('NVDA',   0.110,   10.000000,  220000.00),
        ('META',   0.070,   45.000000,   130000.00),
        ('TSM',    0.085,   200.000000,  160000.00),
        ('ASML',   0.075,    20.000000,  210000.00),
        ('NOVO-B', 0.055,  600.000000,  140000.00),
        ('7203',   0.040,  1800.000000,   120000.00),
        ('005930', 0.020,  6000.000000,   100000.00),
        ('BABA',   0.100,  250.000000,  160000.00)
) v(ticker, weight, quantity, market_value)
  ON s.ticker = v.ticker;

-- ------------------------------------------------------------
-- PRICE HISTORY (seed synthetic prices for ETL: NAV calc)
-- ------------------------------------------------------------

-- ETL uses MAX(price_history.date) <= requested date for each security.
-- We provide a full daily series from 2023-12-01 to 2024-01-15.

WITH tickers AS (
    SELECT id, ticker FROM securities WHERE ticker IN (
        'AAPL','MSFT','AMZN','GOOGL','NVDA','META','TSM','ASML','NOVO-B','7203','005930','BABA'
    )
), base AS (
    -- Reasonable starting adj_close baselines (synthetic but realistic-ish).
    SELECT
        t.id,
        CASE t.ticker
            WHEN 'AAPL'   THEN 190
            WHEN 'MSFT'   THEN 420
            WHEN 'AMZN'   THEN 160
            WHEN 'GOOGL'  THEN 140
            WHEN 'NVDA'   THEN 500
            WHEN 'META'   THEN 350
            WHEN 'TSM'    THEN 120
            WHEN 'ASML'   THEN 650
            WHEN 'NOVO-B' THEN 105
            WHEN '7203'   THEN 210
            WHEN '005930' THEN 74000
            WHEN 'BABA'   THEN 85
            ELSE 100
        END AS base_price,
        CASE t.ticker
            WHEN '005930' THEN 0.020   -- KRW volatility scale
            ELSE 0.010
        END AS vol_scale
    FROM tickers t
), dates AS (
    SELECT d::date AS date
    FROM generate_series(DATE '2023-12-01', DATE '2024-01-15', INTERVAL '1 day') AS g(d)
), price_matrix AS (
    SELECT
        b.id AS security_id,
        dt.date,
        -- day index to create a deterministic mild trend
        (dt.date - DATE '2023-12-01')::int AS day_idx,
        b.base_price,
        b.vol_scale
    FROM base b
    CROSS JOIN dates dt
)
INSERT INTO price_history (
    id, security_id, date, open, high, low, close, adj_close, volume, source
)
SELECT
    uuid_generate_v4()                                                    AS id,
    pm.security_id                                                        AS security_id,
    pm.date                                                               AS date,
    -- Synthetic OHLC based on adj_close trajectory
    (pm.base_price * (1 + pm.day_idx * 0.0015 + (sin(pm.day_idx / 3.0) * pm.vol_scale)))                         AS open,
    (pm.base_price * (1 + pm.day_idx * 0.0015 + (sin(pm.day_idx / 3.0) * pm.vol_scale) + (0.008 * pm.vol_scale))) AS high,
    (pm.base_price * (1 + pm.day_idx * 0.0015 + (sin(pm.day_idx / 3.0) * pm.vol_scale) - (0.008 * pm.vol_scale))) AS low,
    (pm.base_price * (1 + pm.day_idx * 0.0015 + (sin(pm.day_idx / 3.0) * pm.vol_scale) + (0.002 * pm.vol_scale))) AS close,
    (pm.base_price * (1 + pm.day_idx * 0.0015 + (sin(pm.day_idx / 3.0) * pm.vol_scale)))                         AS adj_close,
    -- Volume: just a stable-ish synthetic number
    (CASE
        WHEN (pm.security_id::text LIKE '%00000000-0000-0000-0000-000000000%') THEN 1000000
        ELSE 900000
     END)::bigint                                                       AS volume,
    'seed_synthetic'                                                     AS source
FROM price_matrix pm
ON CONFLICT (security_id, date) DO UPDATE SET
    open      = EXCLUDED.open,
    high      = EXCLUDED.high,
    low       = EXCLUDED.low,
    close     = EXCLUDED.close,
    adj_close = EXCLUDED.adj_close,
    volume    = EXCLUDED.volume,
    fetched_at = NOW();
