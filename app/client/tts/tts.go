package tts

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"durkalive/app/config"

	"github.com/samber/do"
)

type Client struct {
	cfg    *config.Config
	client *http.Client
}

func NewClient(di *do.Injector) (*Client, error) {
	cfg := do.MustInvoke[*config.Config](di)
	return &Client{
		cfg:    cfg,
		client: &http.Client{},
	}, nil
}

func (c *Client) Synthesize(ctx context.Context, text string) ([]byte, error) {
	u, err := url.Parse(c.cfg.TTS.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid TTS base URL: %w", err)
	}
	q := u.Query()
	q.Set("speaker", c.cfg.TTS.Speaker)
	q.Set("text", text)
	q.Set("ext", "wav")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.TTS.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TTS request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("TTS API returned %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}
