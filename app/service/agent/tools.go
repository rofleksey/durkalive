package agent

import (
	"context"
	"durkalive/app/service/memory"
	"encoding/json"
	"fmt"

	"github.com/tmc/langchaingo/tools"
)

type agentTool struct {
	name        string
	description string
	call        func(ctx context.Context, input string) (string, error)
}

func (m *agentTool) Name() string {
	return m.name
}

func (m *agentTool) Description() string {
	return m.description
}

func (m *agentTool) Call(ctx context.Context, input string) (string, error) {
	return m.call(ctx, input)
}

func (s *Service) createMemoryTools() []tools.Tool {
	return []tools.Tool{
		&agentTool{
			name:        "memory_add_facts",
			description: "Add facts to memory. Facts are always assigned to entities. Creates entities if they don't exist. Input must be JSON array of objects with entityName (string) and facts (string[]) fields.",
			call: func(ctx context.Context, input string) (string, error) {
				var requests []memory.AddFactsRequest
				if err := json.Unmarshal([]byte(input), &requests); err != nil {
					return "", fmt.Errorf("invalid requests JSON: %w", err)
				}

				if err := s.memorySvc.AddFacts(requests); err != nil {
					return "", err
				}

				return "ok", nil
			},
		},
		&agentTool{
			name:        "memory_delete_facts",
			description: "Delete facts from memory. Input must be JSON array of objects with entityName (string) and facts ([]string) fields.",
			call: func(ctx context.Context, input string) (string, error) {
				var deletions []memory.DeleteFactsRequest
				if err := json.Unmarshal([]byte(input), &deletions); err != nil {
					return "", fmt.Errorf("invalid deletions JSON: %w", err)
				}

				if err := s.memorySvc.DeleteFacts(deletions); err != nil {
					return "", err
				}

				return "ok", nil
			},
		},
		&agentTool{
			name:        "memory_search",
			description: "Search for entities by exact names. Input must be a JSON array with names.",
			call: func(ctx context.Context, input string) (string, error) {
				var names []string
				if err := json.Unmarshal([]byte(input), &names); err != nil {
					return "", fmt.Errorf("invalid names JSON: %w", err)
				}

				entities, err := s.memorySvc.SearchNodes(names)
				if err != nil {
					return "", err
				}

				result, _ := json.Marshal(entities)
				return string(result), nil
			},
		},
	}
}
