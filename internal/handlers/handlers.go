package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/springstreet/prisma-factsheet/internal/pipeline"
	"github.com/springstreet/prisma-factsheet/internal/services"
)

// Handler holds all service dependencies.
type Handler struct {
	factsheetSvc *services.FactsheetService
	etlRunner    *pipeline.PipelineRunner
}

func New(svc *services.FactsheetService, etl *pipeline.PipelineRunner) *Handler {
	return &Handler{factsheetSvc: svc, etlRunner: etl}
}

// RegisterRoutes attaches all routes to the given gin engine.
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	v1 := r.Group("/api/v1")
	{
		// Products
		v1.GET("/products", h.ListProducts)
		v1.GET("/products/:slug/factsheet", h.GetFactsheet)
		v1.GET("/products/:slug/nav", h.GetNAVSeries)
		v1.GET("/products/:slug/performance", h.GetPerformance)
		v1.GET("/products/:slug/holdings", h.GetHoldings)
		v1.GET("/products/:slug/exposures", h.GetExposures)

		// ETL triggers (protected by admin middleware in production)
		v1.POST("/admin/etl/run", h.TriggerETL)
		v1.GET("/admin/etl/runs", h.ListETLRuns)

		// Health
		r.GET("/health", h.Health)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// PRODUCT HANDLERS
// ─────────────────────────────────────────────────────────────────────────────

// @Summary  List all active products
// @Tags     products
// @Produce  json
// @Success  200 {array} models.Product
// @Router   /api/v1/products [get]
func (h *Handler) ListProducts(c *gin.Context) {
	products, err := h.factsheetSvc.ListProducts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": products, "count": len(products)})
}

// @Summary  Get full factsheet for a product
// @Tags     products
// @Produce  json
// @Param    slug path string true "Product slug"
// @Success  200 {object} models.FactsheetResponse
// @Router   /api/v1/products/{slug}/factsheet [get]
func (h *Handler) GetFactsheet(c *gin.Context) {
	slug := c.Param("slug")
	fs, err := h.factsheetSvc.GetFactsheet(slug)
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error()[:15] == "product not fou" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": fs})
}

// @Summary  Get NAV time series for a product
// @Tags     products
// @Produce  json
// @Param    slug  path  string true  "Product slug"
// @Param    from  query string false "Start date YYYY-MM-DD (default: 1 year ago)"
// @Param    to    query string false "End date YYYY-MM-DD (default: today)"
// @Success  200 {array} models.NAVPoint
// @Router   /api/v1/products/{slug}/nav [get]
func (h *Handler) GetNAVSeries(c *gin.Context) {
	slug := c.Param("slug")
	fromStr := c.DefaultQuery("from", time.Now().AddDate(-1, 0, 0).Format("2006-01-02"))
	toStr := c.DefaultQuery("to", time.Now().Format("2006-01-02"))

	from, _ := time.Parse("2006-01-02", fromStr)
	to, _ := time.Parse("2006-01-02", toStr)
	_ = slug
	_ = from
	_ = to

	// Delegate to factsheetSvc.GetNAVRange (extend service for production)
	c.JSON(http.StatusOK, gin.H{"from": fromStr, "to": toStr, "data": []interface{}{}})
}

// @Summary  Get performance metrics for a product
// @Tags     products
// @Produce  json
// @Param    slug path string true "Product slug"
// @Router   /api/v1/products/{slug}/performance [get]
func (h *Handler) GetPerformance(c *gin.Context) {
	fs, err := h.factsheetSvc.GetFactsheet(c.Param("slug"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": fs.Performance, "as_of_date": fs.AsOfDate})
}

// @Summary  Get current holdings for a product
// @Tags     products
// @Produce  json
// @Param    slug  path  string true  "Product slug"
// @Param    limit query int    false "Max number of holdings (default: 10)"
// @Router   /api/v1/products/{slug}/holdings [get]
func (h *Handler) GetHoldings(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "10")
	limit, _ := strconv.Atoi(limitStr)
	_ = limit

	fs, err := h.factsheetSvc.GetFactsheet(c.Param("slug"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": fs.Holdings, "as_of_date": fs.AsOfDate})
}

// @Summary  Get exposure breakdowns for a product
// @Tags     products
// @Produce  json
// @Param    slug      path  string true  "Product slug"
// @Param    dimension query string false "Filter by: country | sector | market_cap_tier"
// @Router   /api/v1/products/{slug}/exposures [get]
func (h *Handler) GetExposures(c *gin.Context) {
	fs, err := h.factsheetSvc.GetFactsheet(c.Param("slug"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	dimension := c.Query("dimension")
	switch dimension {
	case "country":
		c.JSON(http.StatusOK, gin.H{"data": fs.Exposures.ByCountry, "dimension": "country"})
	case "sector":
		c.JSON(http.StatusOK, gin.H{"data": fs.Exposures.BySector, "dimension": "sector"})
	case "market_cap_tier":
		c.JSON(http.StatusOK, gin.H{"data": fs.Exposures.ByMarketCap, "dimension": "market_cap_tier"})
	default:
		c.JSON(http.StatusOK, gin.H{"data": fs.Exposures})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ADMIN / ETL HANDLERS
// ─────────────────────────────────────────────────────────────────────────────

type TriggerETLRequest struct {
	Pipeline string `json:"pipeline"` // price_fetch | nav_calc | exposure_calc | perf_calc | all
	Date     string `json:"date"`     // YYYY-MM-DD, defaults to today
}

// @Summary  Manually trigger an ETL pipeline
// @Tags     admin
// @Accept   json
// @Produce  json
// @Param    body body TriggerETLRequest true "Pipeline config"
// @Router   /api/v1/admin/etl/run [post]
func (h *Handler) TriggerETL(c *gin.Context) {
	var req TriggerETLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	date := time.Now()
	if req.Date != "" {
		if d, err := time.Parse("2006-01-02", req.Date); err == nil {
			date = d
		}
	}

	var runErr error
	switch req.Pipeline {
	case "price_fetch":
		runErr = h.etlRunner.RunPriceFetch()
	case "nav_calc":
		runErr = h.etlRunner.RunNAVCalc(date)
	case "exposure_calc":
		runErr = h.etlRunner.RunExposureCalc(date)
	case "perf_calc":
		runErr = h.etlRunner.RunPerformanceCalc(date)
	case "all":
		runErr = h.etlRunner.RunPriceFetch()
		if runErr == nil {
			runErr = h.etlRunner.RunNAVCalc(date)
		}
		if runErr == nil {
			runErr = h.etlRunner.RunExposureCalc(date)
		}
		if runErr == nil {
			runErr = h.etlRunner.RunPerformanceCalc(date)
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown pipeline: " + req.Pipeline})
		return
	}

	if runErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": runErr.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "pipeline": req.Pipeline, "date": date.Format("2006-01-02")})
}

// @Summary  List recent ETL runs
// @Tags     admin
// @Produce  json
// @Router   /api/v1/admin/etl/runs [get]
func (h *Handler) ListETLRuns(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "query etl_runs table"})
}

// @Summary  Health check
// @Router   /health [get]
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "prisma-factsheet"})
}