package conversation

import (
	"context"
	"durkalive/app/config"
	"durkalive/app/service/memory"
	"fmt"
	"net/http"
	"strings"
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

func searchAndFormatFacts(ctx context.Context, cfg *config.Config, memorySvc *memory.Service, tags, usernames []string) string {
	facts := memorySvc.Search(ctx, []string{}, tags, usernames, 50)
	if len(facts) == 0 {
		return "Нет фактов"
	}

	var builder strings.Builder
	for _, fact := range facts {
		builder.WriteString("id=")
		builder.WriteString(fmt.Sprint(fact.ID))
		builder.WriteString(", content=")
		builder.WriteString(fact.Content)
		builder.WriteString("\n")
	}

	return builder.String()
}
