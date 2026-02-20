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

var dbFilePath = filepath.Join("data", "facts.json")

type Service struct {
	cfg   *config.Config
	mu    sync.RWMutex
	facts []string
}

func New(di *do.Injector) (*Service, error) {
	_ = os.MkdirAll("data", 0755)

	facts, err := loadFacts()
	if err != nil {
		slog.Warn("Error loading facts", "err", err)
		facts = []string{}
	}

	return &Service{
		cfg:   do.MustInvoke[*config.Config](di),
		facts: facts,
	}, nil
}

func loadFacts() ([]string, error) {
	file, err := os.OpenFile(dbFilePath, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open facts file: %w", err)
	}
	defer file.Close()

	var facts []string
	decoder := json.NewDecoder(file)
	if err = decoder.Decode(&facts); err != nil {
		return nil, fmt.Errorf("failed to decode facts: %w", err)
	}

	return facts, nil
}

func (s *Service) saveFacts() error {
	file, err := os.OpenFile(dbFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create/open facts file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	if err = encoder.Encode(s.facts); err != nil {
		return fmt.Errorf("failed to encode facts: %w", err)
	}

	if err = writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}

	return nil
}

func (s *Service) AddFacts(facts []string) error {
	if len(facts) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	newFacts := make([]string, 0, len(facts))
	existing := make(map[string]bool)

	for _, fact := range s.facts {
		existing[fact] = true
	}

	for _, fact := range facts {
		fact = strings.TrimSpace(fact)
		if fact == "" {
			continue
		}
		if !existing[fact] {
			newFacts = append(newFacts, fact)
			existing[fact] = true
		}
	}

	if len(newFacts) == 0 {
		return nil
	}

	s.facts = append(s.facts, newFacts...)

	if err := s.saveFacts(); err != nil {
		return fmt.Errorf("failed to save facts after addition: %w", err)
	}

	slog.Info("Added facts", "facts", facts, "total", len(s.facts))

	return nil
}

func (s *Service) RemoveFacts(indices []int) error {
	if len(indices) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.facts) == 0 {
		return fmt.Errorf("no facts to remove")
	}

	validIndices := make([]int, 0, len(indices))
	indexMap := make(map[int]bool)

	for _, idx := range indices {
		if idx < 0 || idx >= len(s.facts) {
			return fmt.Errorf("invalid index %d: out of range [0, %d]", idx, len(s.facts)-1)
		}
		if !indexMap[idx] {
			validIndices = append(validIndices, idx)
			indexMap[idx] = true
		}
	}

	for i := 0; i < len(validIndices)-1; i++ {
		for j := i + 1; j < len(validIndices); j++ {
			if validIndices[i] < validIndices[j] {
				validIndices[i], validIndices[j] = validIndices[j], validIndices[i]
			}
		}
	}

	removedFacts := make([]string, 0, len(validIndices))

	for _, idx := range validIndices {
		removedFacts = append(removedFacts, s.facts[idx])
		s.facts = append(s.facts[:idx], s.facts[idx+1:]...)
	}

	if err := s.saveFacts(); err != nil {
		return fmt.Errorf("failed to save facts after removal: %w", err)
	}

	slog.Info("Removed facts",
		"count", len(removedFacts),
		"remaining", len(s.facts),
		"removed", removedFacts)

	return nil
}

func (s *Service) Format() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.facts) == 0 {
		return "No facts"
	}

	var builder strings.Builder
	for i, fact := range s.facts {
		builder.WriteString(fmt.Sprintf("%d - %s\n", i+1, fact))
	}

	return builder.String()
}
