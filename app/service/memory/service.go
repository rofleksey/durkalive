package memory

import (
	"bufio"
	"durkalive/app/config"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/samber/do"
)

var dbFilePath = filepath.Join("data", "memory.json")

type Service struct {
	cfg *config.Config

	mu sync.RWMutex
}

func New(di *do.Injector) (*Service, error) {
	_ = os.MkdirAll("data", 0755)

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
		Entities:  []Entity{},
		Relations: []Relation{},
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var item jsonLineItem
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("failed to parse JSON line: %w", err)
		}

		switch item.Type {
		case "entity":
			graph.Entities = append(graph.Entities, Entity{
				Name:         item.Name,
				EntityType:   item.EntityType,
				Observations: item.Observations,
			})
		case "relation":
			graph.Relations = append(graph.Relations, Relation{
				From:         item.From,
				To:           item.To,
				RelationType: item.RelationType,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading memory file: %w", err)
	}

	return graph, nil
}

func (s *Service) saveGraph(graph *KnowledgeGraph) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Create(dbFilePath)
	if err != nil {
		return fmt.Errorf("failed to create memory file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	for _, e := range graph.Entities {
		item := jsonLineItem{
			Type:         "entity",
			Name:         e.Name,
			EntityType:   e.EntityType,
			Observations: e.Observations,
		}
		data, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("failed to marshal entity: %w", err)
		}
		if _, err = writer.WriteString(string(data) + "\n"); err != nil {
			return fmt.Errorf("failed to write entity: %w", err)
		}
	}

	for _, r := range graph.Relations {
		item := jsonLineItem{
			Type:         "relation",
			From:         r.From,
			To:           r.To,
			RelationType: r.RelationType,
		}
		data, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("failed to marshal relation: %w", err)
		}
		if _, err := writer.WriteString(string(data) + "\n"); err != nil {
			return fmt.Errorf("failed to write relation: %w", err)
		}
	}

	if err = writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}

	return nil
}

func (s *Service) CreateEntities(entities []Entity) ([]Entity, error) {
	graph, err := s.loadGraph()
	if err != nil {
		return nil, err
	}

	var newEntities []Entity
	for _, e := range entities {
		exists := false
		for _, existing := range graph.Entities {
			if existing.Name == e.Name {
				exists = true
				break
			}
		}
		if !exists {
			newEntities = append(newEntities, e)
		}
	}

	if len(newEntities) == 0 {
		return []Entity{}, nil
	}

	graph.Entities = append(graph.Entities, newEntities...)

	if err = s.saveGraph(graph); err != nil {
		return nil, err
	}

	return newEntities, nil
}

func (s *Service) CreateRelations(relations []Relation) ([]Relation, error) {
	graph, err := s.loadGraph()
	if err != nil {
		return nil, err
	}

	var newRelations []Relation
	for _, r := range relations {
		exists := false
		for _, existing := range graph.Relations {
			if existing.From == r.From && existing.To == r.To && existing.RelationType == r.RelationType {
				exists = true
				break
			}
		}
		if !exists {
			newRelations = append(newRelations, r)
		}
	}

	if len(newRelations) == 0 {
		return []Relation{}, nil
	}

	graph.Relations = append(graph.Relations, newRelations...)

	if err = s.saveGraph(graph); err != nil {
		return nil, err
	}

	return newRelations, nil
}

func (s *Service) AddObservations(observations []AddObservationsRequest) ([]AddObservationsResult, error) {
	graph, err := s.loadGraph()
	if err != nil {
		return nil, err
	}

	var results []AddObservationsResult

	for _, o := range observations {
		var entity *Entity
		for i := range graph.Entities {
			if graph.Entities[i].Name == o.EntityName {
				entity = &graph.Entities[i]
				break
			}
		}

		if entity == nil {
			return nil, fmt.Errorf("entity with name %s not found", o.EntityName)
		}

		var newObservations []string
		for _, content := range o.Contents {
			exists := false
			for _, obs := range entity.Observations {
				if obs == content {
					exists = true
					break
				}
			}
			if !exists {
				newObservations = append(newObservations, content)
			}
		}

		if len(newObservations) > 0 {
			entity.Observations = append(entity.Observations, newObservations...)
		}

		results = append(results, AddObservationsResult{
			EntityName:        o.EntityName,
			AddedObservations: newObservations,
		})
	}

	if err := s.saveGraph(graph); err != nil {
		return nil, err
	}

	return results, nil
}

func (s *Service) DeleteEntities(entityNames []string) error {
	graph, err := s.loadGraph()
	if err != nil {
		return err
	}

	namesToDelete := make(map[string]bool)
	for _, name := range entityNames {
		namesToDelete[name] = true
	}

	var newEntities []Entity
	for _, e := range graph.Entities {
		if !namesToDelete[e.Name] {
			newEntities = append(newEntities, e)
		}
	}
	graph.Entities = newEntities

	var newRelations []Relation
	for _, r := range graph.Relations {
		if !namesToDelete[r.From] && !namesToDelete[r.To] {
			newRelations = append(newRelations, r)
		}
	}
	graph.Relations = newRelations

	return s.saveGraph(graph)
}

func (s *Service) DeleteObservations(deletions []DeleteObservationsRequest) error {
	graph, err := s.loadGraph()
	if err != nil {
		return err
	}

	for _, d := range deletions {
		for i := range graph.Entities {
			if graph.Entities[i].Name == d.EntityName {
				// Create a set of observations to delete
				toDelete := make(map[string]bool)
				for _, obs := range d.Observations {
					toDelete[obs] = true
				}

				// Filter out observations
				var newObservations []string
				for _, obs := range graph.Entities[i].Observations {
					if !toDelete[obs] {
						newObservations = append(newObservations, obs)
					}
				}
				graph.Entities[i].Observations = newObservations
				break
			}
		}
	}

	return s.saveGraph(graph)
}

func (s *Service) DeleteRelations(relations []Relation) error {
	graph, err := s.loadGraph()
	if err != nil {
		return err
	}

	// Create a set for O(1) lookup
	type relationKey struct {
		from, to, relationType string
	}
	toDelete := make(map[relationKey]bool)
	for _, r := range relations {
		toDelete[relationKey{r.From, r.To, r.RelationType}] = true
	}

	// Filter out relations to delete
	var newRelations []Relation
	for _, r := range graph.Relations {
		key := relationKey{r.From, r.To, r.RelationType}
		if !toDelete[key] {
			newRelations = append(newRelations, r)
		}
	}
	graph.Relations = newRelations

	return s.saveGraph(graph)
}

func (s *Service) SearchNodes(query string) (*KnowledgeGraph, error) {
	graph, err := s.loadGraph()
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)

	var filteredEntities []Entity
	filteredEntityNames := make(map[string]bool)

	for _, e := range graph.Entities {
		if strings.Contains(strings.ToLower(e.Name), query) {
			filteredEntities = append(filteredEntities, e)
			filteredEntityNames[e.Name] = true
			continue
		}

		if strings.Contains(strings.ToLower(e.EntityType), query) {
			filteredEntities = append(filteredEntities, e)
			filteredEntityNames[e.Name] = true
			continue
		}

		for _, obs := range e.Observations {
			if strings.Contains(strings.ToLower(obs), query) {
				filteredEntities = append(filteredEntities, e)
				filteredEntityNames[e.Name] = true
				break
			}
		}
	}

	var filteredRelations []Relation
	for _, r := range graph.Relations {
		if filteredEntityNames[r.From] && filteredEntityNames[r.To] {
			filteredRelations = append(filteredRelations, r)
		}
	}

	return &KnowledgeGraph{
		Entities:  filteredEntities,
		Relations: filteredRelations,
	}, nil
}

func (s *Service) OpenNodes(names []string) (*KnowledgeGraph, error) {
	graph, err := s.loadGraph()
	if err != nil {
		return nil, err
	}

	namesToInclude := make(map[string]bool)
	for _, name := range names {
		namesToInclude[name] = true
	}

	var filteredEntities []Entity
	for _, e := range graph.Entities {
		if namesToInclude[e.Name] {
			filteredEntities = append(filteredEntities, e)
		}
	}

	includedNames := make(map[string]bool)
	for _, e := range filteredEntities {
		includedNames[e.Name] = true
	}

	var filteredRelations []Relation
	for _, r := range graph.Relations {
		if includedNames[r.From] && includedNames[r.To] {
			filteredRelations = append(filteredRelations, r)
		}
	}

	return &KnowledgeGraph{
		Entities:  filteredEntities,
		Relations: filteredRelations,
	}, nil
}
