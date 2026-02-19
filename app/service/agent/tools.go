package agent

import (
	"context"
	"durkalive/app/service/memory"
	"encoding/json"
	"fmt"
	"strings"

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
			name:        "memory_create_entities",
			description: "Create new entities in the knowledge graph. Input must be JSON array of entities with name (string), entityType (string), and observations (string[]) fields.",
			call: func(ctx context.Context, input string) (string, error) {
				var entities []memory.Entity
				if err := json.Unmarshal([]byte(input), &entities); err != nil {
					return "", fmt.Errorf("invalid entities JSON: %w", err)
				}

				created, err := s.memorySvc.CreateEntities(entities)
				if err != nil {
					return "", err
				}

				result, _ := json.Marshal(created)
				return string(result), nil
			},
		},
		&agentTool{
			name:        "memory_create_relations",
			description: "Create new relations between EXISTING entities. Input must be JSON array of relations with from, to, and relationType fields (all strings).",
			call: func(ctx context.Context, input string) (string, error) {
				var relations []memory.Relation
				if err := json.Unmarshal([]byte(input), &relations); err != nil {
					return "", fmt.Errorf("invalid relations JSON: %w", err)
				}

				created, err := s.memorySvc.CreateRelations(relations)
				if err != nil {
					return "", err
				}

				result, _ := json.Marshal(created)
				return string(result), nil
			},
		},
		&agentTool{
			name:        "memory_add_observations",
			description: "Add observations to EXISTING entities. Input must be JSON array of AddObservationsRequest with entityName (string) and contents (string[]) fields.",
			call: func(ctx context.Context, input string) (string, error) {
				var observations []memory.AddObservationsRequest
				if err := json.Unmarshal([]byte(input), &observations); err != nil {
					return "", fmt.Errorf("invalid observations JSON: %w", err)
				}

				results, err := s.memorySvc.AddObservations(observations)
				if err != nil {
					return "", err
				}

				result, _ := json.Marshal(results)
				return string(result), nil
			},
		},
		&agentTool{
			name:        "memory_delete_entities",
			description: "Delete entities by name. Input must be JSON array of entity names.",
			call: func(ctx context.Context, input string) (string, error) {
				var names []string
				if err := json.Unmarshal([]byte(input), &names); err != nil {
					return "", fmt.Errorf("invalid entity names JSON: %w", err)
				}

				if err := s.memorySvc.DeleteEntities(names); err != nil {
					return "", err
				}

				return "Entities deleted successfully", nil
			},
		},
		&agentTool{
			name:        "memory_delete_observations",
			description: "Delete observations from EXISTING entities. Input must be JSON array of objects with entityName (string) and observations ([]string) fields.",
			call: func(ctx context.Context, input string) (string, error) {
				var deletions []memory.DeleteObservationsRequest
				if err := json.Unmarshal([]byte(input), &deletions); err != nil {
					return "", fmt.Errorf("invalid deletions JSON: %w", err)
				}

				if err := s.memorySvc.DeleteObservations(deletions); err != nil {
					return "", err
				}

				return "Observations deleted successfully", nil
			},
		},
		&agentTool{
			name:        "memory_delete_relations",
			description: "Delete EXISTING relations. Input must be JSON array of relations with from, to, and relationType fields (all strings).",
			call: func(ctx context.Context, input string) (string, error) {
				var relations []memory.Relation
				if err := json.Unmarshal([]byte(input), &relations); err != nil {
					return "", fmt.Errorf("invalid relations JSON: %w", err)
				}

				if err := s.memorySvc.DeleteRelations(relations); err != nil {
					return "", err
				}

				return "Relations deleted successfully", nil
			},
		},
		&agentTool{
			name:        "memory_search_nodes",
			description: "Search for nodes containing the query string. Input must be a JSON string with the query.",
			call: func(ctx context.Context, input string) (string, error) {
				var query string
				if err := json.Unmarshal([]byte(input), &query); err != nil {
					// Try raw string if JSON parsing fails
					query = strings.Trim(input, "\"")
				}

				graph, err := s.memorySvc.SearchNodes(query)
				if err != nil {
					return "", err
				}

				result, _ := json.Marshal(graph)
				return string(result), nil
			},
		},
		&agentTool{
			name:        "memory_open_nodes",
			description: "Retrieve specific nodes by their names. Input must be JSON array of node names.",
			call: func(ctx context.Context, input string) (string, error) {
				var names []string
				if err := json.Unmarshal([]byte(input), &names); err != nil {
					return "", fmt.Errorf("invalid node names JSON: %w", err)
				}

				graph, err := s.memorySvc.OpenNodes(names)
				if err != nil {
					return "", err
				}

				result, _ := json.Marshal(graph)
				return string(result), nil
			},
		},
	}
}
