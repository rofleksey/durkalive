package transcribe

import (
	"context"
	"durkalive/app/client/speechkit"
	"durkalive/app/config"
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
	cfg    *config.Config
	client *speechkit.YandexSpeechKit
}

func New(di *do.Injector) (*Service, error) {
	return &Service{
		cfg:    do.MustInvoke[*config.Config](di),
		client: do.MustInvoke[*speechkit.YandexSpeechKit](di),
	}, nil
}

func (s *Service) Start(ctx context.Context, m3u8URL string) *Context {
	transCtx := NewContext(ctx)

	go s.runTranscription(transCtx, m3u8URL)

	return transCtx
}

func (s *Service) runTranscription(transCtx *Context, m3u8URL string) {
	defer close(transCtx.phraseChan)
	defer transCtx.Cancel(nil)

	ffmpeg, err := NewFFmpegStream(transCtx, m3u8URL)
	if err != nil {
		transCtx.Cancel(fmt.Errorf("failed to create ffmpeg stream: %w", err))
		return
	}

	if err = ffmpeg.Start(); err != nil {
		transCtx.Cancel(fmt.Errorf("failed to start ffmpeg: %w", err))
		return
	}
	defer ffmpeg.Stop()

	audioStream := ffmpeg.GetAudioStream()

	g, ctx := errgroup.WithContext(transCtx)

	g.Go(func() error {
		return s.runTranscriptionWithRetry(ctx, audioStream, transCtx)
	})

	err = g.Wait()
	if err != nil && !errors.Is(err, context.Canceled) {
		transCtx.Cancel(err)
		slog.Error("transcription failed", "error", err)
	}
}

func (s *Service) runTranscriptionWithRetry(ctx context.Context, audioSrc io.Reader, transCtx *Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := s.runSingleTranscription(ctx, audioSrc, transCtx)
			if err == nil {
				return nil
			}

			if errors.Is(err, io.EOF) {
				slog.Info("received EOF from speechkit, restarting client")
				continue
			}

			return fmt.Errorf("transcription error: %w", err)
		}
	}
}

func (s *Service) runSingleTranscription(ctx context.Context, audioSrc io.Reader, transCtx *Context) error {
	handle, err := s.client.Start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transcription: %w", err)
	}
	defer handle.Close()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return s.streamAudio(ctx, audioSrc, handle)
	})

	g.Go(func() error {
		return s.receivePhrases(handle, transCtx)
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

func (s *Service) receivePhrases(handle *speechkit.Handle, transCtx *Context) error {
	for {
		sentences, err := handle.Recv()
		if err != nil {
			return fmt.Errorf("Recv: %w", err)
		}

		for _, text := range sentences {
			transCtx.sendPhrase(text)
		}
	}
}
