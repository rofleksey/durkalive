package transcribe

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"runtime"
	"sync"
)

type FFmpegStream struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser
	mu     sync.Mutex
}

func NewMicStream(ctx context.Context, micDevice string) (*FFmpegStream, error) {
	var args []string
	switch runtime.GOOS {
	case "windows":
		device := micDevice
		if device == "" {
			device = "Microphone"
		}
		// Single argument so spaces in device name are preserved (no shell)
		args = []string{
			"-loglevel", "warning",
			"-f", "dshow",
			"-i", "audio=" + device,
			"-acodec", "pcm_s16le",
			"-ac", "1",
			"-ar", "48000",
			"-f", "wav",
			"-",
		}
	default:
		device := micDevice
		if device == "" {
			device = "default"
		}
		args = []string{
			"-loglevel", "warning",
			"-f", "pulse",
			"-i", device,
			"-acodec", "pcm_s16le",
			"-ac", "1",
			"-ar", "48000",
			"-f", "wav",
			"-",
		}
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	slog.Info("Running ffmpeg from microphone", "device", micDevice)

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
