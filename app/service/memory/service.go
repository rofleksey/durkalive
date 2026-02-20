package memory

import (
	"bufio"
	"durkalive/app/config"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/samber/do"
)

var dbFilePath = filepath.Join("data", "memory.json")

type Service struct {
	cfg *config.Config
	mu  sync.RWMutex
}

func New(di *do.Injector) (*Service, error) {
	_ = os.MkdirAll("data", 0755)

	file, err := os.Create(dbFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create memory file: %w", err)
	}
	defer file.Close()

	return &Service{
		cfg: do.MustInvoke[*config.Config](di),
	}, nil
}

func (s *Service) loadGraph() (*KnowledgeGraph, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	file, err := os.OpenFile(dbFilePath, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open memory file: %w", err)
	}
	defer file.Close()

	graph := &KnowledgeGraph{
		Entities: []*Entity{},
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var item jsonLineItem
		if err = json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("failed to parse JSON line: %w", err)
		}

		graph.Entities = append(graph.Entities, &Entity{
			Name:  item.Name,
			Facts: item.Facts,
		})
	}

	if err = scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading memory file: %w", err)
	}

	return graph, nil
}

func (s *Service) saveGraph(graph *KnowledgeGraph) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.OpenFile(dbFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create/open memory file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	for _, e := range graph.Entities {
		item := jsonLineItem{
			Name:  e.Name,
			Facts: e.Facts,
		}
		data, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("failed to marshal entity: %w", err)
		}
		if _, err = writer.WriteString(string(data) + "\n"); err != nil {
			return fmt.Errorf("failed to write entity: %w", err)
		}
	}

	if err = writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}

	return nil
}

func (s *Service) AddFacts(requests []AddFactsRequest) error {
	graph, err := s.loadGraph()
	if err != nil {
		return err
	}

	for _, req := range requests {
		var entity *Entity

		for i := range graph.Entities {
			if graph.Entities[i].Name == req.EntityName {
				entity = graph.Entities[i]
				break
			}
		}

		if entity == nil {
			graph.Entities = append(graph.Entities, &Entity{
				Name:  req.EntityName,
				Facts: req.Facts,
			})
		} else {
			var newFacts []string
			for _, content := range req.Facts {
				exists := false
				for _, obs := range entity.Facts {
					if obs == content {
						exists = true
						break
					}
				}
				if !exists {
					newFacts = append(newFacts, content)
				}
			}

			if len(newFacts) > 0 {
				entity.Facts = append(entity.Facts, newFacts...)
			}
		}
	}

	if err = s.saveGraph(graph); err != nil {
		return err
	}

	slog.Info("Created facts", "facts", requests)

	return nil
}

func (s *Service) DeleteFacts(deletions []DeleteFactsRequest) error {
	graph, err := s.loadGraph()
	if err != nil {
		return err
	}

	totalDeleted := 0
	for _, d := range deletions {
		for i := range graph.Entities {
			if graph.Entities[i].Name == d.EntityName {
				toDelete := make(map[string]bool)
				for _, obs := range d.Facts {
					toDelete[obs] = true
				}

				var newFacts []string
				for _, obs := range graph.Entities[i].Facts {
					if !toDelete[obs] {
						newFacts = append(newFacts, obs)
					} else {
						totalDeleted++
					}
				}
				graph.Entities[i].Facts = newFacts
				break
			}
		}
	}

	if err = s.saveGraph(graph); err != nil {
		return err
	}

	slog.Info("Deleted facts", "deletions", deletions)
	return nil
}

func (s *Service) SearchNodes(names []string) ([]*Entity, error) {
	graph, err := s.loadGraph()
	if err != nil {
		return nil, err
	}

	result := make([]*Entity, 0)

	for _, name := range names {
		name = strings.ToLower(name)

		for _, e := range graph.Entities {
			if strings.ToLower(e.Name) == name {
				result = append(result, e)
				continue
			}
		}
	}

	slog.Info("Search completed",
		"names", names,
		"entities_count", len(result),
	)

	return result, nil
}
