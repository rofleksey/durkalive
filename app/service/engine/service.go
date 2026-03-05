package engine

import (
	"context"
	"durkalive/app/config"
	"durkalive/app/service/conversation"
	"durkalive/app/service/queue"
	"durkalive/app/service/transcribe"
	"log/slog"
	"time"

	"github.com/samber/do"
)

type Service struct {
	cfg             *config.Config
	transcribeSvc   *transcribe.Service
	conversationSvc *conversation.Service
	queueSvc        *queue.Service
}

func New(di *do.Injector) (*Service, error) {
	return &Service{
		cfg:             do.MustInvoke[*config.Config](di),
		transcribeSvc:   do.MustInvoke[*transcribe.Service](di),
		conversationSvc: do.MustInvoke[*conversation.Service](di),
		queueSvc:        do.MustInvoke[*queue.Service](di),
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
			time.Sleep(time.Minute)
		}
	}
}

func (s *Service) runIteration(ctx context.Context) error {
	transcribeCtx, cancel := s.transcribeSvc.Start(ctx)
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
			if err := s.conversationSvc.ProcessMessage(ctx, msg.Username, msg.Text); err != nil {
				slog.Warn("ProcessMessage error", "error", err)
			}

			slog.Debug("Processed message",
				"username", msg.Username,
				"text", msg.Text,
				"duration", time.Since(start))
		}
	}
}
