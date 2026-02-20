package engine

import (
	"context"
	"durkalive/app/client/twitch_live"
	"durkalive/app/config"
	"durkalive/app/service/agent"
	"durkalive/app/service/queue"
	"durkalive/app/service/transcribe"
	"fmt"
	"log/slog"
	"time"

	"github.com/elliotchance/pie/v2"
	"github.com/samber/do"
)

type Service struct {
	cfg           *config.Config
	liveClient    *twitch_live.Client
	transcribeSvc *transcribe.Service
	agentSvc      *agent.Service
	queueSvc      *queue.Service
}

func New(di *do.Injector) (*Service, error) {
	return &Service{
		cfg:           do.MustInvoke[*config.Config](di),
		liveClient:    do.MustInvoke[*twitch_live.Client](di),
		transcribeSvc: do.MustInvoke[*transcribe.Service](di),
		agentSvc:      do.MustInvoke[*agent.Service](di),
		queueSvc:      do.MustInvoke[*queue.Service](di),
	}, nil
}

func (s *Service) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := s.runIteration(ctx); err != nil {
			slog.Error("Error running iteration", "error", err)
		}
	}
}

func (s *Service) runIteration(ctx context.Context) error {
	qualities, err := s.liveClient.GetM3U8(ctx, s.cfg.Twitch.Channel)
	if err != nil {
		return fmt.Errorf("could not get qualities: %w", err)
	}

	qualityIndex := pie.FindFirstUsing(qualities, func(q twitch_live.StreamQuality) bool {
		return q.Quality == "audio_only"
	})
	if qualityIndex < 0 {
		qualityIndex = 0
	}

	streamQuality := qualities[qualityIndex]
	streamURL := streamQuality.URL

	transcribeCtx, cancel := s.transcribeSvc.Start(ctx, streamURL)
	defer cancel(nil)

	for {
		select {
		case <-transcribeCtx.Done():
			return context.Cause(transcribeCtx)
		case msg, ok := <-s.queueSvc.Channel():
			if !ok {
				return context.Canceled
			}

			start := time.Now()
			if err = s.agentSvc.ProcessMessage(ctx, msg.Username, msg.Text); err != nil {
				slog.Warn("ProcessMessage error", "error", err)
			}

			slog.Info("Processed message",
				"username", msg.Username,
				"text", msg.Text,
				"duration", time.Since(start))
		}
	}
}
