package pipeline

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/springstreet/prisma-factsheet/internal/models"
)

const (
	yahooBaseURL = "https://query1.finance.yahoo.com/v8/finance/chart"
)

// YahooFetcher fetches OHLCV data from the Yahoo Finance v8 chart API.
type YahooFetcher struct {
	client *http.Client
}

func NewYahooFetcher() *YahooFetcher {
	return &YahooFetcher{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// yahooChartResponse mirrors the Yahoo Finance /v8/finance/chart JSON shape.
type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol             string  `json:"symbol"`
				Currency           string  `json:"currency"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []float64 `json:"open"`
					High   []float64 `json:"high"`
					Low    []float64 `json:"low"`
					Close  []float64 `json:"close"`
					Volume []int64   `json:"volume"`
				} `json:"quote"`
				Adjclose []struct {
					Adjclose []float64 `json:"adjclose"`
				} `json:"adjclose"`
			} `json:"indicators"`
		} `json:"result"`
		Error interface{} `json:"error"`
	} `json:"chart"`
}

// FetchHistory retrieves daily OHLCV prices for a ticker between start and end.
func (yf *YahooFetcher) FetchHistory(ticker string, start, end time.Time) ([]models.PriceHistory, error) {
	url := fmt.Sprintf(
		"%s/%s?interval=1d&period1=%d&period2=%d&events=history",
		yahooBaseURL, ticker,
		start.Unix(), end.Unix(),
	)

	log.Debug().Str("ticker", ticker).Str("url", url).Msg("fetching yahoo finance")

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; PrismaBot/1.0)")
	req.Header.Set("Accept", "application/json")

	resp, err := yf.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yahoo fetch %s: %w", ticker, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body %s: %w", ticker, err)
	}

	var raw yahooChartResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", ticker, err)
	}

	if len(raw.Chart.Result) == 0 {
		return nil, fmt.Errorf("no data for %s", ticker)
	}

	result := raw.Chart.Result[0]
	quotes := result.Indicators.Quote
	adjclose := result.Indicators.Adjclose

	if len(quotes) == 0 {
		return nil, fmt.Errorf("empty quotes for %s", ticker)
	}

	n := len(result.Timestamp)
	prices := make([]models.PriceHistory, 0, n)

	for i := 0; i < n; i++ {
		if i >= len(quotes[0].Close) || quotes[0].Close[i] == 0 {
			continue
		}
		adj := quotes[0].Close[i]
		if len(adjclose) > 0 && i < len(adjclose[0].Adjclose) && adjclose[0].Adjclose[i] != 0 {
			adj = adjclose[0].Adjclose[i]
		}
		prices = append(prices, models.PriceHistory{
			Date:     time.Unix(result.Timestamp[i], 0).UTC(),
			Open:     safeIdx(quotes[0].Open, i),
			High:     safeIdx(quotes[0].High, i),
			Low:      safeIdx(quotes[0].Low, i),
			Close:    quotes[0].Close[i],
			AdjClose: adj,
			Volume:   safeIdxInt(quotes[0].Volume, i),
			Source:   "yahoo_finance",
		})
	}

	log.Info().Str("ticker", ticker).Int("bars", len(prices)).Msg("fetched price history")
	return prices, nil
}

// FetchLatestPrice returns only the most recent closing price.
func (yf *YahooFetcher) FetchLatestPrice(ticker string) (float64, time.Time, error) {
	end := time.Now()
	start := end.AddDate(0, 0, -5)
	prices, err := yf.FetchHistory(ticker, start, end)
	if err != nil || len(prices) == 0 {
		return 0, time.Time{}, fmt.Errorf("fetch latest %s: %w", ticker, err)
	}
	latest := prices[len(prices)-1]
	return latest.AdjClose, latest.Date, nil
}

func safeIdx(s []float64, i int) float64 {
	if i < len(s) {
		return s[i]
	}
	return 0
}

func safeIdxInt(s []int64, i int) int64 {
	if i < len(s) {
		return s[i]
	}
	return 0
}