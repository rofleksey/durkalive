package main

import (
	"context"
	"durkalive/app/client/speechkit"
	"durkalive/app/client/twitch"
	"durkalive/app/client/twitch_irc"
	"durkalive/app/client/twitch_live"
	"durkalive/app/config"
	"durkalive/app/service/conversation"
	"durkalive/app/service/engine"
	"durkalive/app/service/memory"
	"durkalive/app/service/queue"
	"durkalive/app/service/transcribe"
	"durkalive/app/util/mylog"
	"log/slog"
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
	do.Provide(di, twitch.NewClient)
	do.Provide(di, twitch_live.NewClient)
	do.Provide(di, twitch_irc.NewClient)
	do.Provide(di, transcribe.New)
	do.Provide(di, memory.New)
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

	go do.MustInvoke[*twitch.Client](di).RunRefreshLoop(appCtx)
	go do.MustInvoke[*twitch_irc.Client](di).RunRefreshLoop(appCtx)

	go do.MustInvoke[*engine.Service](di).Run(appCtx)

	<-appCtx.Done()
}
