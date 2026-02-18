package transcribe

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
)

type FFmpegStream struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser
	mu     sync.Mutex
}

func NewFFmpegStream(ctx context.Context, m3u8URL string) (*FFmpegStream, error) {
	args := []string{
		"-loglevel", "warning",
		"-i", m3u8URL,
		"-reconnect", "0",
		"-reconnect_at_eof", "0",
		"-reconnect_streamed", "0",
		"-reconnect_delay_max", "0",
		"-vn",
		"-acodec", "pcm_s16le",
		"-ac", "1",
		"-ar", "16000",
		"-f", "wav",
		"-",
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	slog.Info("Running ffmpeg", "cmd", "ffmpeg "+strings.Join(args, " "))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	return &FFmpegStream{
		cmd:    cmd,
		stdout: stdout,
		stderr: stderr,
	}, nil
}

func (f *FFmpegStream) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	go f.logStderr()

	return nil
}

func (f *FFmpegStream) GetAudioStream() io.ReadCloser {
	return f.stdout
}

func (f *FFmpegStream) Wait() error {
	return f.cmd.Wait()
}

func (f *FFmpegStream) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.cmd.Process != nil {
		return f.cmd.Process.Kill()
	}
	return nil
}

func (f *FFmpegStream) logStderr() {
	scanner := bufio.NewScanner(f.stderr)
	for scanner.Scan() {
		slog.Debug("ffmpeg", "stderr", scanner.Text())
	}
}
