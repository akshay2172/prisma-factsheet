package services

import (
	"fmt"
	"time"

	"github.com/springstreet/prisma-factsheet/internal/db"
	"github.com/springstreet/prisma-factsheet/internal/models"
)

// FactsheetService reads data from DB and assembles the FactsheetResponse.
type FactsheetService struct {
	db *db.DB
}

func NewFactsheetService(database *db.DB) *FactsheetService {
	return &FactsheetService{db: database}
}

// GetFactsheet returns the full factsheet for a product slug, as of the latest available date.
func (s *FactsheetService) GetFactsheet(slug string) (*models.FactsheetResponse, error) {
	// 1. Load product
	var product models.Product
	err := s.db.Get(&product, `SELECT * FROM products WHERE slug = $1 AND is_active = true`, slug)
	if err != nil {
		return nil, fmt.Errorf("product not found: %w", err)
	}

	// 2. Latest NAV
	var latestNAV models.NAVHistory
	err = s.db.Get(&latestNAV, `
		SELECT * FROM nav_history
		WHERE product_id = $1
		ORDER BY date DESC LIMIT 1
	`, product.ID)
	if err != nil {
		return nil, fmt.Errorf("nav not found: %w", err)
	}

	// 3. Performance metrics (latest)
	var perf models.PerformanceMetrics
	_ = s.db.Get(&perf, `
		SELECT * FROM performance_metrics
		WHERE product_id = $1
		ORDER BY as_of_date DESC LIMIT 1
	`, product.ID)

	// 4. Top 10 holdings
	var holdings []models.Holding
	_ = s.db.Select(&holdings, `
		SELECT h.weight, h.market_value,
		       s.ticker, s.name AS security_name, s.sector, s.country_code
		FROM holdings h
		JOIN securities s ON s.id = h.security_id
		WHERE h.product_id = $1
		  AND h.as_of_date = (
		    SELECT MAX(as_of_date) FROM holdings WHERE product_id = $1
		  )
		ORDER BY h.weight DESC
		LIMIT 10
	`, product.ID)

	topHoldings := make([]models.HoldingSummary, len(holdings))
	for i, h := range holdings {
		topHoldings[i] = models.HoldingSummary{
			Ticker:      derefStr(h.SecurityTicker),
			Name:        derefStr(h.SecurityName),
			Weight:      h.Weight,
			Sector:      derefStr(h.Sector),
			CountryCode: derefStr(h.CountryCode),
			MarketValue: derefFloat(h.MarketValue),
		}
	}

	// 5. Exposure breakdown
	exposures, _ := s.getExposures(product.ID, latestNAV.Date)

	// 6. NAV series (last 1 year, weekly sampled)
	navSeries, _ := s.getNAVSeries(product.ID, latestNAV.Date)

	return &models.FactsheetResponse{
		Product:     product,
		AsOfDate:    latestNAV.Date.Format("2006-01-02"),
		NAV:         latestNAV.NAV,
		Performance: perf,
		Holdings:    topHoldings,
		Exposures:   exposures,
		NAVSeries:   navSeries,
	}, nil
}

func (s *FactsheetService) getExposures(productID interface{}, date time.Time) (models.ExposureBreakdown, error) {
	var rows []models.ExposureSnapshot
	err := s.db.Select(&rows, `
		SELECT dimension, label, weight
		FROM exposure_snapshots
		WHERE product_id = $1
		  AND as_of_date = (
		    SELECT MAX(as_of_date) FROM exposure_snapshots WHERE product_id = $1
		  )
		ORDER BY weight DESC
	`, productID)
	if err != nil {
		return models.ExposureBreakdown{}, err
	}

	var breakdown models.ExposureBreakdown
	for _, r := range rows {
		item := models.ExposureItem{Label: r.Label, Weight: r.Weight}
		switch r.Dimension {
		case "country":
			breakdown.ByCountry = append(breakdown.ByCountry, item)
		case "sector":
			breakdown.BySector = append(breakdown.BySector, item)
		case "market_cap_tier":
			breakdown.ByMarketCap = append(breakdown.ByMarketCap, item)
		}
	}
	return breakdown, nil
}

func (s *FactsheetService) getNAVSeries(productID interface{}, latest time.Time) ([]models.NAVPoint, error) {
	oneYearAgo := latest.AddDate(-1, 0, 0)
	var navs []models.NAVHistory
	err := s.db.Select(&navs, `
		SELECT date, nav FROM nav_history
		WHERE product_id = $1 AND date >= $2
		ORDER BY date ASC
	`, productID, oneYearAgo)
	if err != nil {
		return nil, err
	}

	points := make([]models.NAVPoint, len(navs))
	for i, n := range navs {
		points[i] = models.NAVPoint{
			Date: n.Date.Format("2006-01-02"),
			NAV:  n.NAV,
		}
	}
	return points, nil
}

// ListProducts returns all active products (for the product listing endpoint).
func (s *FactsheetService) ListProducts() ([]models.Product, error) {
	var products []models.Product
	err := s.db.Select(&products, `SELECT * FROM products WHERE is_active = true ORDER BY name`)
	return products, err
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefFloat(f *float64) float64 {
	if f == nil {
		return 0.0
	}
	return *f
}