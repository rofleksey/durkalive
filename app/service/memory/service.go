package memory

import (
	"context"
	"durkalive/app/config"
	"durkalive/app/database"
	"durkalive/app/service/embedding"
	"log/slog"
	"strings"

	"github.com/samber/do"
)

const similarityThreshold = 0.75

type Service struct {
	cfg          *config.Config
	db           *database.Service
	embeddingSvc *embedding.Service
}

func New(di *do.Injector) (*Service, error) {
	cfg := do.MustInvoke[*config.Config](di)
	return &Service{
		cfg:          cfg,
		db:           do.MustInvoke[*database.Service](di),
		embeddingSvc: do.MustInvoke[*embedding.Service](di),
	}, nil
}

func (s *Service) AddFact(ctx context.Context, text string, tags []string, usernames []string) {
	slogger := slog.With(
		"text", text,
		"tags", strings.Join(tags, ","),
		"usernames", strings.Join(usernames, ","),
	)

	embeddingVec, err := s.embeddingSvc.CreateEmbedding(ctx, text)
	if err != nil {
		slogger.Error("Failed to create embedding for fact", "error", err)
		return
	}

	similar, err := s.db.FindSimilarFacts(ctx, usernames, embeddingVec, similarityThreshold, 1)
	if err != nil {
		slogger.Error("Failed to check for similar facts", "error", err)
		return
	}

	if len(similar) > 0 {
		_ = s.db.RemoveFact(ctx, similar[0].ID)
		return
	}

	if _, err = s.db.AddFact(ctx, text, tags, usernames, embeddingVec); err != nil {
		slogger.Error("Failed to add fact", "error", err)
		return
	}

	slogger.Info("Added fact")
}

func (s *Service) RemoveFacts(ctx context.Context, ids []int) {
	if len(ids) == 0 {
		return
	}

	for _, id := range ids {
		if err := s.db.RemoveFact(ctx, id); err != nil {
			slog.Error("Failed to remove fact",
				"id", id,
				"error", err,
			)
		}
	}

	slog.Info("Removed facts", "ids", ids)
}

func (s *Service) Search(ctx context.Context, requiredTags, anyTags, usernames []string, limit int) []Fact {
	facts, err := s.db.SearchFacts(ctx, requiredTags, anyTags, usernames, limit)
	if err != nil {
		slog.Error("Failed to search facts", "error", err)
		return nil
	}

	result := make([]Fact, 0, len(facts))
	for _, fact := range facts {
		result = append(result, Fact{
			ID:      fact.ID,
			Content: fact.Content,
		})
	}

	return result
}
