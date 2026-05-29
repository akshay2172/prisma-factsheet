package pipeline

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/springstreet/prisma-factsheet/internal/db"
	"github.com/springstreet/prisma-factsheet/internal/models"
)

// PipelineRunner orchestrates all ETL jobs.
type PipelineRunner struct {
	db      *db.DB
	fetcher *YahooFetcher
}

func NewPipelineRunner(database *db.DB) *PipelineRunner {
	return &PipelineRunner{
		db:      database,
		fetcher: NewYahooFetcher(),
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 1. PRICE FETCH PIPELINE
// ─────────────────────────────────────────────────────────────────────────────

// RunPriceFetch fetches the latest prices for all active securities and upserts them.
func (p *PipelineRunner) RunPriceFetch() error {
	runID := p.startRun("price_fetch")
	var processed int

	// Fetch all securities
	var securities []models.Security
	if err := p.db.Select(&securities, `SELECT * FROM securities`); err != nil {
		return p.failRun(runID, fmt.Errorf("fetch securities: %w", err))
	}

	end := time.Now()
	start := end.AddDate(0, 0, -7) // Last 7 days; daily job only needs 1-2

	for _, sec := range securities {
		prices, err := p.fetcher.FetchHistory(sec.Ticker, start, end)
		if err != nil {
			log.Warn().Err(err).Str("ticker", sec.Ticker).Msg("price fetch failed, skipping")
			continue
		}

		for _, price := range prices {
			_, err := p.db.Exec(`
				INSERT INTO price_history
					(id, security_id, date, open, high, low, close, adj_close, volume, source)
				VALUES
					($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (security_id, date) DO UPDATE SET
					open      = EXCLUDED.open,
					high      = EXCLUDED.high,
					low       = EXCLUDED.low,
					close     = EXCLUDED.close,
					adj_close = EXCLUDED.adj_close,
					volume    = EXCLUDED.volume,
					fetched_at = NOW()
			`,
				uuid.New(), sec.ID, price.Date,
				price.Open, price.High, price.Low, price.Close, price.AdjClose, price.Volume,
				price.Source,
			)
			if err != nil {
				log.Warn().Err(err).Str("ticker", sec.Ticker).Msg("upsert price failed")
				continue
			}
			processed++
		}
	}

	return p.successRun(runID, processed)
}

// ─────────────────────────────────────────────────────────────────────────────
// 2. NAV CALCULATION PIPELINE
// ─────────────────────────────────────────────────────────────────────────────

// RunNAVCalc recomputes NAV for all products for the given date based on
// holdings weights × latest adj_close prices.
func (p *PipelineRunner) RunNAVCalc(date time.Time) error {
	runID := p.startRun("nav_calc")
	var processed int

	var products []models.Product
	if err := p.db.Select(&products, `SELECT * FROM products WHERE is_active = true`); err != nil {
		return p.failRun(runID, fmt.Errorf("fetch products: %w", err))
	}

	for _, prod := range products {
		nav, err := p.calcNAV(prod.ID, date)
		if err != nil {
			log.Warn().Err(err).Str("product", prod.Slug).Msg("NAV calc failed")
			continue
		}

		// Get previous NAV for daily return
		var prevNAV float64
		prevDate := date.AddDate(0, 0, -1)
		err = p.db.QueryRow(
			`SELECT nav FROM nav_history WHERE product_id=$1 AND date=$2`,
			prod.ID, prevDate,
		).Scan(&prevNAV)

		dailyReturn := 0.0
		if err == nil && prevNAV > 0 {
			dailyReturn = (nav-prevNAV)/prevNAV
		}

		_, err = p.db.Exec(`
			INSERT INTO nav_history (id, product_id, date, nav, daily_return)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (product_id, date) DO UPDATE SET
				nav          = EXCLUDED.nav,
				daily_return = EXCLUDED.daily_return
		`, uuid.New(), prod.ID, date, nav, dailyReturn)

		if err != nil {
			log.Warn().Err(err).Str("product", prod.Slug).Msg("upsert NAV failed")
			continue
		}
		processed++
	}

	return p.successRun(runID, processed)
}

// calcNAV computes weighted NAV: Σ (weight_i × price_i). Base NAV = 1000.
func (p *PipelineRunner) calcNAV(productID uuid.UUID, date time.Time) (float64, error) {
	type weightedPrice struct {
		Weight   float64 `db:"weight"`
		AdjClose float64 `db:"adj_close"`
	}

	var rows []weightedPrice
	err := p.db.Select(&rows, `
		SELECT h.weight, ph.adj_close
		FROM holdings h
		JOIN price_history ph ON ph.security_id = h.security_id
		WHERE h.product_id = $1
		  AND h.as_of_date = (
		    SELECT MAX(as_of_date) FROM holdings WHERE product_id = $1 AND as_of_date <= $2
		  )
		  AND ph.date = (
		    SELECT MAX(date) FROM price_history WHERE security_id = h.security_id AND date <= $2
		  )
	`, productID, date)
	if err != nil || len(rows) == 0 {
		return 0, fmt.Errorf("no holdings/prices for NAV: %w", err)
	}

	var totalWeightedReturn float64
	for _, r := range rows {
		totalWeightedReturn += r.Weight * r.AdjClose
	}
	return totalWeightedReturn, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 3. EXPOSURE CALCULATION PIPELINE
// ─────────────────────────────────────────────────────────────────────────────

// RunExposureCalc aggregates holding weights by country, sector, market_cap_tier.
func (p *PipelineRunner) RunExposureCalc(date time.Time) error {
	runID := p.startRun("exposure_calc")
	var processed int

	var products []models.Product
	if err := p.db.Select(&products, `SELECT * FROM products WHERE is_active = true`); err != nil {
		return p.failRun(runID, fmt.Errorf("fetch products: %w", err))
	}

	for _, prod := range products {
		type holdingRow struct {
			Weight      float64 `db:"weight"`
			CountryCode string  `db:"country_code"`
			Sector      string  `db:"sector"`
			MarketCap   float64 `db:"market_value"`
		}

		var rows []holdingRow
		err := p.db.Select(&rows, `
			SELECT h.weight, s.country_code, s.sector, COALESCE(h.market_value, 0) as market_value
			FROM holdings h
			JOIN securities s ON s.id = h.security_id
			WHERE h.product_id = $1
			  AND h.as_of_date = (
			    SELECT MAX(as_of_date) FROM holdings WHERE product_id = $1 AND as_of_date <= $2
			  )
		`, prod.ID, date)

		if err != nil || len(rows) == 0 {
			continue
		}

		// Aggregate by country
		country := map[string]float64{}
		sector := map[string]float64{}
		capTier := map[string]float64{}

		for _, r := range rows {
			country[r.CountryCode] += r.Weight
			sector[r.Sector] += r.Weight
			capTier[marketCapTier(r.MarketCap)] += r.Weight
		}

		for label, wt := range country {
			if label == "" {
				label = "Other"
			}
			p.upsertExposure(prod.ID, date, "country", label, wt)
			processed++
		}
		for label, wt := range sector {
			if label == "" {
				label = "Other"
			}
			p.upsertExposure(prod.ID, date, "sector", label, wt)
			processed++
		}
		for label, wt := range capTier {
			p.upsertExposure(prod.ID, date, "market_cap_tier", label, wt)
			processed++
		}
	}

	return p.successRun(runID, processed)
}

func (p *PipelineRunner) upsertExposure(productID uuid.UUID, date time.Time, dimension, label string, weight float64) {
	_, err := p.db.Exec(`
		INSERT INTO exposure_snapshots (id, product_id, as_of_date, dimension, label, weight)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT DO NOTHING
	`, uuid.New(), productID, date, dimension, label, weight)
	if err != nil {
		log.Warn().Err(err).Str("label", label).Msg("upsert exposure failed")
	}
}

func marketCapTier(mv float64) string {
	switch {
	case mv >= 200_000_000_000:
		return "Mega Cap (>$200B)"
	case mv >= 10_000_000_000:
		return "Large Cap ($10B-$200B)"
	case mv >= 2_000_000_000:
		return "Mid Cap ($2B-$10B)"
	case mv >= 300_000_000:
		return "Small Cap ($300M-$2B)"
	default:
		return "Micro Cap (<$300M)"
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 4. PERFORMANCE METRICS PIPELINE
// ─────────────────────────────────────────────────────────────────────────────

// RunPerformanceCalc computes all performance metrics for a product up to date.
func (p *PipelineRunner) RunPerformanceCalc(date time.Time) error {
	runID := p.startRun("perf_calc")
	var processed int

	var products []models.Product
	if err := p.db.Select(&products, `SELECT * FROM products WHERE is_active = true`); err != nil {
		return p.failRun(runID, fmt.Errorf("fetch products: %w", err))
	}

	for _, prod := range products {
		m, err := p.calcPerformance(prod, date)
		if err != nil {
			log.Warn().Err(err).Str("product", prod.Slug).Msg("perf calc failed")
			continue
		}

		_, err = p.db.Exec(`
			INSERT INTO performance_metrics (
				id, product_id, as_of_date,
				return_1d, return_1w, return_1m, return_3m, return_6m, return_ytd, return_1y, return_3y, return_inception,
				volatility_1y, sharpe_ratio_1y, max_drawdown_1y, beta_1y,
				benchmark_return_1y, alpha_1y
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
			ON CONFLICT (product_id, as_of_date) DO UPDATE SET
				return_1d=EXCLUDED.return_1d, return_1w=EXCLUDED.return_1w,
				return_1m=EXCLUDED.return_1m, return_3m=EXCLUDED.return_3m,
				return_6m=EXCLUDED.return_6m, return_ytd=EXCLUDED.return_ytd,
				return_1y=EXCLUDED.return_1y, return_3y=EXCLUDED.return_3y,
				return_inception=EXCLUDED.return_inception,
				volatility_1y=EXCLUDED.volatility_1y, sharpe_ratio_1y=EXCLUDED.sharpe_ratio_1y,
				max_drawdown_1y=EXCLUDED.max_drawdown_1y, beta_1y=EXCLUDED.beta_1y,
				benchmark_return_1y=EXCLUDED.benchmark_return_1y, alpha_1y=EXCLUDED.alpha_1y
		`,
			uuid.New(), prod.ID, date,
			m.Return1D, m.Return1W, m.Return1M, m.Return3M, m.Return6M, m.ReturnYTD, m.Return1Y, m.Return3Y, m.ReturnInception,
			m.Volatility1Y, m.SharpeRatio1Y, m.MaxDrawdown1Y, m.Beta1Y,
			m.BenchmarkRet1Y, m.Alpha1Y,
		)
		if err != nil {
			log.Warn().Err(err).Str("product", prod.Slug).Msg("upsert perf failed")
			continue
		}
		processed++
	}

	return p.successRun(runID, processed)
}

func (p *PipelineRunner) calcPerformance(prod models.Product, date time.Time) (models.PerformanceMetrics, error) {
	m := models.PerformanceMetrics{ProductID: prod.ID, AsOfDate: date}

	type navRow struct {
		Date        time.Time `db:"date"`
		NAV         float64   `db:"nav"`
		DailyReturn float64   `db:"daily_return"`
	}

	var navs []navRow
	err := p.db.Select(&navs, `
		SELECT date, nav, COALESCE(daily_return, 0) as daily_return
		FROM nav_history
		WHERE product_id = $1 AND date <= $2
		ORDER BY date ASC
	`, prod.ID, date)
	if err != nil || len(navs) == 0 {
		return m, fmt.Errorf("no nav data: %w", err)
	}

	currentNAV := navs[len(navs)-1].NAV
	inceptionNAV := navs[0].NAV

	lookupNAV := func(daysBack int) float64 {
		target := date.AddDate(0, 0, -daysBack)
		for i := len(navs) - 1; i >= 0; i-- {
			if !navs[i].Date.After(target) {
				return navs[i].NAV
			}
		}
		return navs[0].NAV
	}

	returnFrom := func(baseNAV float64) float64 {
		if baseNAV == 0 {
			return 0
		}
		return (currentNAV - baseNAV) / baseNAV
	}

	m.Return1D = returnFrom(lookupNAV(1))
	m.Return1W = returnFrom(lookupNAV(7))
	m.Return1M = returnFrom(lookupNAV(30))
	m.Return3M = returnFrom(lookupNAV(90))
	m.Return6M = returnFrom(lookupNAV(180))
	m.Return1Y = returnFrom(lookupNAV(365))
	m.Return3Y = returnFrom(lookupNAV(1095))
	m.ReturnInception = returnFrom(inceptionNAV)

	// YTD: from Jan 1 of current year
	jan1 := time.Date(date.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	var ytdNAV float64
	for i := len(navs) - 1; i >= 0; i-- {
		if !navs[i].Date.Before(jan1) {
			continue
		}
		ytdNAV = navs[i].NAV
		break
	}
	if ytdNAV == 0 && len(navs) > 0 {
		ytdNAV = navs[0].NAV
	}
	m.ReturnYTD = returnFrom(ytdNAV)

	// Volatility: annualised std of daily returns (last 252 trading days ~1Y)
	returns1Y := []float64{}
	cutoff := date.AddDate(-1, 0, 0)
	for _, n := range navs {
		if n.Date.After(cutoff) {
			returns1Y = append(returns1Y, n.DailyReturn)
		}
	}
	if len(returns1Y) > 1 {
		m.Volatility1Y = annualisedVol(returns1Y)
		meanReturn := mean(returns1Y) * 252
		riskFreeRate := 0.05 // approximate
		if m.Volatility1Y != 0 {
			m.SharpeRatio1Y = (meanReturn - riskFreeRate) / m.Volatility1Y
		}
		m.MaxDrawdown1Y = maxDrawdown(returns1Y)
	}

	m.BenchmarkRet1Y = 0.12 // TODO: calc from benchmark price history
	m.Alpha1Y = m.Return1Y - m.BenchmarkRet1Y
	m.Beta1Y = 1.05 // TODO: calc from rolling covariance

	return m, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────────

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func annualisedVol(dailyReturns []float64) float64 {
	m := mean(dailyReturns)
	variance := 0.0
	for _, r := range dailyReturns {
		d := r - m
		variance += d * d
	}
	variance /= float64(len(dailyReturns) - 1)
	return math.Sqrt(variance) * math.Sqrt(252)
}

func maxDrawdown(dailyReturns []float64) float64 {
	peak := 1.0
	nav := 1.0
	maxDD := 0.0
	for _, r := range dailyReturns {
		nav *= (1 + r)
		if nav > peak {
			peak = nav
		}
		dd := (peak - nav) / peak
		if dd > maxDD {
			maxDD = dd
		}
	}
	return -maxDD
}

// ─────────────────────────────────────────────────────────────────────────────
// ETL RUN TRACKING
// ─────────────────────────────────────────────────────────────────────────────

func (p *PipelineRunner) startRun(pipeline string) uuid.UUID {
	id := uuid.New()
	p.db.Exec(
		`INSERT INTO etl_runs (id, pipeline, status) VALUES ($1, $2, 'running')`,
		id, pipeline,
	)
	log.Info().Str("pipeline", pipeline).Str("run_id", id.String()).Msg("ETL run started")
	return id
}

func (p *PipelineRunner) successRun(id uuid.UUID, processed int) error {
	p.db.Exec(
		`UPDATE etl_runs SET status='success', finished_at=NOW(), records_processed=$1 WHERE id=$2`,
		processed, id,
	)
	log.Info().Str("run_id", id.String()).Int("processed", processed).Msg("ETL run success")
	return nil
}

func (p *PipelineRunner) failRun(id uuid.UUID, err error) error {
	p.db.Exec(
		`UPDATE etl_runs SET status='failed', finished_at=NOW(), error_message=$1 WHERE id=$2`,
		err.Error(), id,
	)
	log.Error().Err(err).Str("run_id", id.String()).Msg("ETL run failed")
	return err
}