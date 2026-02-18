package main

import (
	"context"
	"durkalive/app/client/speechkit"
	"durkalive/app/client/twitch"
	"durkalive/app/client/twitch_live"
	"durkalive/app/config"
	"durkalive/app/service/agent"
	"durkalive/app/service/transcribe"
	"durkalive/app/util/mylog"
	"log/slog"
	"os"
	"os/signal"

	"github.com/elliotchance/pie/v2"
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
	do.Provide(di, transcribe.New)
	do.Provide(di, agent.New)

	slog.Info("Service started")

	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint

		log.Info("Shutting down server...")

		cancel()
	}()

	go do.MustInvoke[*twitch.Client](di).RunRefreshLoop(appCtx)

	agentSvc := do.MustInvoke[*agent.Service](di)

	liveClient := do.MustInvoke[*twitch_live.Client](di)
	qualities, err := liveClient.GetM3U8(appCtx, cfg.Twitch.Channel)
	if err != nil {
		log.Fatalf("get qualities failed: %v", err)
	}

	qualityIndex := pie.FindFirstUsing(qualities, func(q twitch_live.StreamQuality) bool {
		return q.Quality == "audio_only"
	})
	if qualityIndex < 0 {
		qualityIndex = 0
	}

	streamQuality := qualities[qualityIndex]
	streamURL := streamQuality.URL

	transcribeSvc := do.MustInvoke[*transcribe.Service](di)
	transcribeCtx := transcribeSvc.Start(appCtx, streamURL)

	for sentence := range transcribeCtx.GetPhraseChannel() {
		slog.Info("Sentence", "text", sentence)
		if err = agentSvc.ReactStreamerMessage(appCtx, sentence); err != nil {
			slog.Warn("ReactStreamerMessage error", "error", err)
		}
	}
	<-transcribeCtx.Done()

	if transcribeCtx.Err() != nil {
		slog.Error("Transcribe failed", "err", transcribeCtx.Err())
	}

	//<-appCtx.Done()
}
