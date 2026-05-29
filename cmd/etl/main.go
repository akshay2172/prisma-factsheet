package main

import (
	"flag"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/springstreet/prisma-factsheet/internal/db"
	"github.com/springstreet/prisma-factsheet/internal/pipeline"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	_ = godotenv.Load()

	job := flag.String("job", "all", "ETL job to run: price_fetch | nav_calc | exposure_calc | perf_calc | all")
	dateStr := flag.String("date", time.Now().Format("2006-01-02"), "Target date YYYY-MM-DD")
	flag.Parse()

	date, err := time.Parse("2006-01-02", *dateStr)
	if err != nil {
		log.Fatal().Err(err).Msg("invalid date")
	}

	database, err := db.Connect()
	if err != nil {
		log.Fatal().Err(err).Msg("db connect failed")
	}
	defer database.Close()

	runner := pipeline.NewPipelineRunner(database)

	switch *job {
	case "price_fetch":
		err = runner.RunPriceFetch()
	case "nav_calc":
		err = runner.RunNAVCalc(date)
	case "exposure_calc":
		err = runner.RunExposureCalc(date)
	case "perf_calc":
		err = runner.RunPerformanceCalc(date)
	case "all":
		if err = runner.RunPriceFetch(); err != nil {
			break
		}
		if err = runner.RunNAVCalc(date); err != nil {
			break
		}
		if err = runner.RunExposureCalc(date); err != nil {
			break
		}
		err = runner.RunPerformanceCalc(date)
	default:
		log.Fatal().Str("job", *job).Msg("unknown job")
	}

	if err != nil {
		log.Fatal().Err(err).Str("job", *job).Msg("ETL job failed")
	}
	log.Info().Str("job", *job).Msg("ETL job completed successfully")
}