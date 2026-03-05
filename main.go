package main

import (
	"context"
	"durkalive/app/client/speechkit"
	"durkalive/app/client/tts"
	"durkalive/app/config"
	"durkalive/app/database"
	"durkalive/app/service/conversation"
	"durkalive/app/service/embedding"
	"durkalive/app/service/engine"
	"durkalive/app/service/memory"
	"durkalive/app/service/playback"
	"durkalive/app/service/queue"
	"durkalive/app/service/recentmemory"
	"durkalive/app/service/transcribe"
	"durkalive/app/util/mylog"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"

	"github.com/gofiber/fiber/v2/log"
	"github.com/samber/do"
)

func main() {
	di := do.New()
	defer di.Shutdown()
	defer log.Info("Waiting for services to finish...")

	mylog.Preinit()

	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	do.ProvideValue(di, appCtx)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}
	do.ProvideValue(di, cfg)

	if err = mylog.Init(cfg); err != nil {
		log.Fatalf("logging init failed: %v", err)
	}

	do.Provide(di, speechkit.NewClient)
	do.Provide(di, tts.NewClient)
	do.Provide(di, transcribe.New)
	do.Provide(di, database.New)
	do.Provide(di, memory.New)
	do.Provide(di, recentmemory.New)
	do.Provide(di, embedding.New)
	do.Provide(di, playback.New)
	do.Provide(di, conversation.New)
	do.Provide(di, queue.New)
	do.Provide(di, engine.New)

	slog.Info("Service started")

	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint

		log.Info("Shutting down...")

		cancel()
	}()

	go func() {
		playbackSvc := do.MustInvoke[*playback.Service](di)
		if err := playbackSvc.Run(appCtx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Playback server error", "error", err)
		}
	}()

	go do.MustInvoke[*engine.Service](di).Run(appCtx)

	<-appCtx.Done()
}
