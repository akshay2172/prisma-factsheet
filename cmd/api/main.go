package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/springstreet/prisma-factsheet/internal/db"
	"github.com/springstreet/prisma-factsheet/internal/handlers"
	"github.com/springstreet/prisma-factsheet/internal/pipeline"
	"github.com/springstreet/prisma-factsheet/internal/services"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	_ = godotenv.Load()

	database, err := db.Connect()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer database.Close()

	factsheetSvc := services.NewFactsheetService(database)
	etlRunner := pipeline.NewPipelineRunner(database)

	c := cron.New()
	// Daily at 01:00 UTC (06:30 IST) — after global market close
	c.AddFunc("0 1 * * *", func() {
		now := time.Now().UTC()
		log.Info().Msg("cron: starting daily ETL pipeline")
		if err := etlRunner.RunPriceFetch(); err != nil {
			log.Error().Err(err).Msg("cron: price fetch failed")
			return
		}
		_ = etlRunner.RunNAVCalc(now)
		_ = etlRunner.RunExposureCalc(now)
		_ = etlRunner.RunPerformanceCalc(now)
		log.Info().Msg("cron: daily ETL complete")
	})
	c.Start()
	defer c.Stop()

	if os.Getenv("APP_ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), corsMiddleware())

	h := handlers.New(factsheetSvc, etlRunner)
	h.RegisterRoutes(router)

	port := getEnv("PORT", "8080")
	log.Info().Str("port", port).Msg("Prisma Factsheet API listening")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if err := router.Run(":" + port); err != nil {
			log.Fatal().Err(err).Msg("server error")
		}
	}()
	<-quit
	log.Info().Msg("graceful shutdown")
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}