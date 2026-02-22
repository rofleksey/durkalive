package memory

import (
	"durkalive/app/config"
	"durkalive/app/database"
	"log/slog"
	"strings"

	"github.com/samber/do"
)

type Service struct {
	cfg *config.Config
	db  *database.Service
}

func New(di *do.Injector) (*Service, error) {
	return &Service{
		cfg: do.MustInvoke[*config.Config](di),
		db:  do.MustInvoke[*database.Service](di),
	}, nil
}

func (s *Service) AddFact(text string, tags []string, usernames []string) {
	slogger := slog.With(
		"text", text,
		"tags", strings.Join(tags, ","),
	)

	if _, err := s.db.AddFact(text, tags, usernames); err != nil {
		slogger.Error("Failed to add fact", "error", err)
		return
	}

	slogger.Info("Added fact")
}

func (s *Service) RemoveFacts(ids []int) {
	if len(ids) == 0 {
		return
	}

	for _, id := range ids {
		if err := s.db.RemoveFact(id); err != nil {
			slog.Error("Failed to remove fact",
				"id", id,
				"error", err,
			)
		}
	}

	slog.Info("Removed facts", "ids", ids)
}

func (s *Service) Search(requiredTags, anyTags, usernames []string, limit int) []Fact {
	facts, err := s.db.SearchFacts(requiredTags, anyTags, usernames, limit)
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
