package embedding

import (
	"context"
	"durkalive/app/config"
	"fmt"
	"net/http"
	"time"

	"github.com/pgvector/pgvector-go"
	"github.com/samber/do"
	"github.com/sashabaranov/go-openai"
)

type Service struct {
	cfg    *config.Config
	client *openai.Client
}

func New(di *do.Injector) (*Service, error) {
	cfg := do.MustInvoke[*config.Config](di)

	clientConfig := openai.DefaultConfig(cfg.OpenAI.Embedding.Token)
	clientConfig.BaseURL = cfg.OpenAI.Embedding.BaseURL
	clientConfig.HTTPClient = &http.Client{
		Timeout: 30 * time.Second,
	}
	return &Service{
		cfg:    cfg,
		client: openai.NewClientWithConfig(clientConfig),
	}, nil
}

func (s *Service) CreateEmbedding(ctx context.Context, text string) (pgvector.Vector, error) {
	resp, err := s.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Input: []string{text},
		Model: openai.EmbeddingModel(s.cfg.OpenAI.Embedding.Model),
	})
	if err != nil {
		return pgvector.Vector{}, fmt.Errorf("failed to create embedding: %w", err)
	}

	if len(resp.Data) == 0 {
		return pgvector.Vector{}, fmt.Errorf("no embedding data returned")
	}

	return pgvector.NewVector(resp.Data[0].Embedding), nil
}
