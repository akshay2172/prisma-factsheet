package models

import (
	"time"

	"github.com/google/uuid"
)

// ─── Product ─────────────────────────────────────────────────────────────────

type Product struct {
	ID            uuid.UUID  `db:"id"             json:"id"`
	Slug          string     `db:"slug"           json:"slug"`
	Name          string     `db:"name"           json:"name"`
	Description   string     `db:"description"    json:"description"`
	ProductType   string     `db:"product_type"   json:"product_type"`
	Currency      string     `db:"currency"       json:"currency"`
	InceptionDate *time.Time `db:"inception_date" json:"inception_date"`
	BenchmarkID   *uuid.UUID `db:"benchmark_id"   json:"benchmark_id"`
	IsActive      bool       `db:"is_active"      json:"is_active"`
	CreatedAt     time.Time  `db:"created_at"     json:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at"     json:"updated_at"`
}

// ─── Security ────────────────────────────────────────────────────────────────

type Security struct {
	ID          uuid.UUID `db:"id"          json:"id"`
	Ticker      string    `db:"ticker"      json:"ticker"`
	Exchange    *string   `db:"exchange"    json:"exchange"`
	ISIN        *string   `db:"isin"        json:"isin"`
	Name        string    `db:"name"        json:"name"`
	AssetClass  string    `db:"asset_class" json:"asset_class"`
	CountryCode *string   `db:"country_code" json:"country_code"`
	Sector      *string   `db:"sector"      json:"sector"`
	Industry    *string   `db:"industry"    json:"industry"`
	Currency    string    `db:"currency"    json:"currency"`
	CreatedAt   time.Time `db:"created_at"  json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"  json:"updated_at"`
}

// ─── Holding ─────────────────────────────────────────────────────────────────

type Holding struct {
	ID          uuid.UUID `db:"id"           json:"id"`
	ProductID   uuid.UUID `db:"product_id"   json:"product_id"`
	SecurityID  uuid.UUID `db:"security_id"  json:"security_id"`
	AsOfDate    time.Time `db:"as_of_date"   json:"as_of_date"`
	Weight      float64   `db:"weight"       json:"weight"`
	Quantity    *float64  `db:"quantity"     json:"quantity"`
	MarketValue *float64  `db:"market_value" json:"market_value"`
	CreatedAt   time.Time `db:"created_at"   json:"created_at"`
	// Joined fields
	SecurityTicker  *string `db:"ticker"       json:"ticker,omitempty"`
	SecurityName    *string `db:"security_name" json:"security_name,omitempty"`
	Sector          *string `db:"sector"       json:"sector,omitempty"`
	CountryCode     *string `db:"country_code" json:"country_code,omitempty"`
}

// ─── Price ────────────────────────────────────────────────────────────────────

type PriceHistory struct {
	ID         uuid.UUID `db:"id"          json:"id"`
	SecurityID uuid.UUID `db:"security_id" json:"security_id"`
	Date       time.Time `db:"date"        json:"date"`
	Open       float64   `db:"open"        json:"open"`
	High       float64   `db:"high"        json:"high"`
	Low        float64   `db:"low"         json:"low"`
	Close      float64   `db:"close"       json:"close"`
	AdjClose   float64   `db:"adj_close"   json:"adj_close"`
	Volume     int64     `db:"volume"      json:"volume"`
	Source     string    `db:"source"      json:"source"`
	FetchedAt  time.Time `db:"fetched_at"  json:"fetched_at"`
}

// ─── NAV ─────────────────────────────────────────────────────────────────────

type NAVHistory struct {
	ID          uuid.UUID `db:"id"           json:"id"`
	ProductID   uuid.UUID `db:"product_id"   json:"product_id"`
	Date        time.Time `db:"date"         json:"date"`
	NAV         float64   `db:"nav"          json:"nav"`
	DailyReturn float64   `db:"daily_return" json:"daily_return"`
	CreatedAt   time.Time `db:"created_at"   json:"created_at"`
}

// ─── Performance ─────────────────────────────────────────────────────────────

type PerformanceMetrics struct {
	ID              uuid.UUID `db:"id"                   json:"id"`
	ProductID       uuid.UUID `db:"product_id"           json:"product_id"`
	AsOfDate        time.Time `db:"as_of_date"           json:"as_of_date"`
	Return1D        float64   `db:"return_1d"            json:"return_1d"`
	Return1W        float64   `db:"return_1w"            json:"return_1w"`
	Return1M        float64   `db:"return_1m"            json:"return_1m"`
	Return3M        float64   `db:"return_3m"            json:"return_3m"`
	Return6M        float64   `db:"return_6m"            json:"return_6m"`
	ReturnYTD       float64   `db:"return_ytd"           json:"return_ytd"`
	Return1Y        float64   `db:"return_1y"            json:"return_1y"`
	Return3Y        float64   `db:"return_3y"            json:"return_3y"`
	ReturnInception float64   `db:"return_inception"     json:"return_inception"`
	Volatility1Y    float64   `db:"volatility_1y"        json:"volatility_1y"`
	SharpeRatio1Y   float64   `db:"sharpe_ratio_1y"      json:"sharpe_ratio_1y"`
	MaxDrawdown1Y   float64   `db:"max_drawdown_1y"      json:"max_drawdown_1y"`
	Beta1Y          float64   `db:"beta_1y"              json:"beta_1y"`
	BenchmarkRet1Y  float64   `db:"benchmark_return_1y"  json:"benchmark_return_1y"`
	Alpha1Y         float64   `db:"alpha_1y"             json:"alpha_1y"`
	CreatedAt       time.Time `db:"created_at"           json:"created_at"`
}

// ─── Exposure ─────────────────────────────────────────────────────────────────

type ExposureSnapshot struct {
	ID        uuid.UUID `db:"id"         json:"id"`
	ProductID uuid.UUID `db:"product_id" json:"product_id"`
	AsOfDate  time.Time `db:"as_of_date" json:"as_of_date"`
	Dimension string    `db:"dimension"  json:"dimension"` // country | sector | market_cap_tier
	Label     string    `db:"label"      json:"label"`
	Weight    float64   `db:"weight"     json:"weight"`
}

// ─── ETL ──────────────────────────────────────────────────────────────────────

type ETLRun struct {
	ID               uuid.UUID  `db:"id"                json:"id"`
	Pipeline         string     `db:"pipeline"          json:"pipeline"`
	Status           string     `db:"status"            json:"status"`
	StartedAt        time.Time  `db:"started_at"        json:"started_at"`
	FinishedAt       *time.Time `db:"finished_at"       json:"finished_at"`
	RecordsProcessed int        `db:"records_processed" json:"records_processed"`
	ErrorMessage     string     `db:"error_message"     json:"error_message"`
}

// ─── API Response Types ───────────────────────────────────────────────────────

// FactsheetResponse is the full factsheet API response, assembled from multiple tables.
type FactsheetResponse struct {
	Product     Product            `json:"product"`
	AsOfDate    string             `json:"as_of_date"`
	NAV         float64            `json:"nav"`
	Performance PerformanceMetrics `json:"performance"`
	Holdings    []HoldingSummary   `json:"top_holdings"`
	Exposures   ExposureBreakdown  `json:"exposures"`
	NAVSeries   []NAVPoint         `json:"nav_series"`
}

type HoldingSummary struct {
	Ticker      string  `json:"ticker"`
	Name        string  `json:"name"`
	Weight      float64 `json:"weight"`
	Sector      string  `json:"sector"`
	CountryCode string  `json:"country_code"`
	MarketValue float64 `json:"market_value"`
}

type ExposureBreakdown struct {
	ByCountry   []ExposureItem `json:"by_country"`
	BySector    []ExposureItem `json:"by_sector"`
	ByMarketCap []ExposureItem `json:"by_market_cap"`
}

type ExposureItem struct {
	Label  string  `json:"label"`
	Weight float64 `json:"weight"`
}

type NAVPoint struct {
	Date string  `json:"date"`
	NAV  float64 `json:"nav"`
}