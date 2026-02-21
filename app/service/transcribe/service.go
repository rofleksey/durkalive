package transcribe

import (
	"context"
	"durkalive/app/client/speechkit"
	"durkalive/app/client/twitch_irc"
	"durkalive/app/config"
	"durkalive/app/service/queue"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/samber/do"
	"golang.org/x/sync/errgroup"
)

const (
	bufferSize = 4096
)

type Service struct {
	cfg          *config.Config
	speechClient *speechkit.YandexSpeechKit
	ircClient    *twitch_irc.Client
	queue        *queue.Service
}

func New(di *do.Injector) (*Service, error) {
	return &Service{
		cfg:          do.MustInvoke[*config.Config](di),
		speechClient: do.MustInvoke[*speechkit.YandexSpeechKit](di),
		ircClient:    do.MustInvoke[*twitch_irc.Client](di),
		queue:        do.MustInvoke[*queue.Service](di),
	}, nil
}

func (s *Service) Start(ctx context.Context, m3u8URL string) (context.Context, context.CancelCauseFunc) {
	ctx, cancel := context.WithCancelCause(ctx)

	go s.runTranscription(ctx, cancel, m3u8URL)

	return ctx, cancel
}

func (s *Service) runTranscription(ctx context.Context, cancel context.CancelCauseFunc, m3u8URL string) {
	defer cancel(nil)

	ffmpeg, err := NewFFmpegStream(ctx, m3u8URL)
	if err != nil {
		cancel(fmt.Errorf("failed to create ffmpeg stream: %w", err))
		return
	}

	if err = ffmpeg.Start(); err != nil {
		cancel(fmt.Errorf("failed to start ffmpeg: %w", err))
		return
	}
	defer ffmpeg.Stop()

	audioStream := ffmpeg.GetAudioStream()

	go func() {
		cancel(s.runTranscriptionWithRetry(ctx, audioStream))
	}()

	go func() {
		s.ircClient.JoinChannel(s.cfg.Twitch.Channel)
		s.ircClient.SetListener(func(channel, username, messageID, text string, tags map[string]string) {
			if s.cfg.Twitch.IgnoreChat {
				return
			}

			s.queue.Add(username, text)
		})

		cancel(s.ircClient.Run())
	}()
	defer s.ircClient.Disconnect()

	go func() {
		err := ffmpeg.Wait()
		if err == nil {
			err = fmt.Errorf("ffmpeg process finished")
		}
		cancel(err)
	}()

	<-ctx.Done()

	if err = context.Cause(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("Transcription failed", "error", err)
	}
}

func (s *Service) runTranscriptionWithRetry(ctx context.Context, audioSrc io.Reader) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := s.runSingleTranscription(ctx, audioSrc)
			if err == nil {
				return nil
			}

			if errors.Is(err, io.EOF) {
				slog.Info("received EOF from speechkit, restarting speechClient")
				continue
			}

			return fmt.Errorf("transcription error: %w", err)
		}
	}
}

func (s *Service) runSingleTranscription(ctx context.Context, audioSrc io.Reader) error {
	handle, err := s.speechClient.Start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transcription: %w", err)
	}
	defer handle.Close()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return s.streamAudio(ctx, audioSrc, handle)
	})

	g.Go(func() error {
		return s.receivePhrases(ctx, handle)
	})

	return g.Wait()
}

func (s *Service) streamAudio(ctx context.Context, audioSrc io.Reader, handle *speechkit.Handle) error {
	if err := handle.SendConfig(); err != nil {
		return fmt.Errorf("failed to send audio config: %w", err)
	}

	buffer := make([]byte, bufferSize)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			n, err := audioSrc.Read(buffer)
			if err != nil {
				return fmt.Errorf("failed to read audio: %w", err)
			}

			if n == 0 {
				continue
			}

			if err = handle.Send(buffer[:n]); err != nil {
				return fmt.Errorf("failed to send audio: %w", err)
			}
		}
	}
}

func (s *Service) receivePhrases(ctx context.Context, handle *speechkit.Handle) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		sentences, err := handle.Recv()
		if err != nil {
			return fmt.Errorf("Recv: %w", err)
		}

		for _, text := range sentences {
			s.queue.Add(s.cfg.Twitch.Channel, text)
		}
	}
}
