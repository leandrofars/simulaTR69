package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/localhots/SimulaTR69/datamodel"
	"github.com/localhots/SimulaTR69/simulator"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.DateTime,
	})
	if err := simulator.LoadConfig(ctx); err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}
	cfg := simulator.Config

	log.Info().Str("file", cfg.DataModelPath).Msg("Loading datamodel")
	defaults, err := datamodel.LoadDataModelFile(cfg.DataModelPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load datamodel")
	}
	if cfg.NormalizeParameters {
		datamodel.NormalizeParameters(defaults)
	}

	log.Info().Int("sim_number", cfg.SimNumber).Msg("Starting simulators")

	var wg sync.WaitGroup
	wg.Add(cfg.SimNumber)

	for i := 0; i < cfg.SimNumber; i++ {

		log.Debug().Str("file", cfg.StateFilePath).Msg("Loading state")
		state, err := datamodel.LoadState(cfg.StateFilePath)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to load state")
		}

		dm := datamodel.New(state.WithDefaults(defaults))

		if cfg.SerialNumber != "" {
			dm.SetSerialNumber(cfg.SerialNumber + "-" + strconv.Itoa(i))
		}

		id := dm.DeviceID()
		log.Debug().
			Str("manufacturer", id.Manufacturer).
			Str("oui", id.OUI).
			Str("product_class", id.ProductClass).
			Str("serial_number", id.SerialNumber).
			Msg("Simulating device")

		// used when migrating tr-069 device to usp
		// param, ok := dm.GetValue("Device.LocalAgent.EndpointID")
		// if ok {
		// 	dm.SetValue("Device.LocalAgent.EndpointID", param.Value+"-"+strconv.Itoa(i))
		// }

		srv := simulator.New(dm, simulator.Config.ConnectionRequestPort+uint16(i), simulator.Config.ConnReqHost)
		go func() {
			// defer srv.Stop(ctx)

			// FIXME: something's off with error checking here
			// nolint:errorlint
			if err := srv.Start(ctx); err != nil && err != http.ErrServerClosed {
				slog.Error("Failed to start simulator", "error", err)
			}

			<-ctx.Done()

			log.Debug().Str("id", id.SerialNumber).Msg("Stopping simulated device")
			srv.Stop(ctx)
			wg.Done()

			// TODO: should we stop the server ? srv.Stop(ctx)
		}()

		if i > 0 && i%cfg.DevicePerSecond == 0 {
			time.Sleep(time.Second)
		}
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	cancel()
	wg.Wait()

	log.Info().Msg("All simulators stopped, shutting down")
}
