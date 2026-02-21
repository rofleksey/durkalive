package conversation

import (
	"durkalive/app/config"
	"net/http"
	"time"

	"github.com/sashabaranov/go-openai"
)

func createClient(cfg config.ModelConfig) *openai.Client {
	clientConfig := openai.DefaultConfig(cfg.Token)

	clientConfig.BaseURL = cfg.BaseURL
	clientConfig.HTTPClient = &http.Client{
		Timeout: 30 * time.Second,
	}

	return openai.NewClientWithConfig(clientConfig)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "никогда"
	}

	return t.Format("15:04:05")
}
