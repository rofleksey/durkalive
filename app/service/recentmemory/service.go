package recentmemory

import (
	"durkalive/app/config"
	"strings"
	"sync"

	"github.com/samber/do"
)

type Service struct {
	mu      sync.RWMutex
	entries []string
	max     int
}

func New(di *do.Injector) (*Service, error) {
	cfg := do.MustInvoke[*config.Config](di)

	return &Service{
		entries: make([]string, 0, cfg.Conversation.RecentMemoryMaxEntries),
		max:     cfg.Conversation.RecentMemoryMaxEntries,
	}, nil
}

func (s *Service) Add(entry string) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
	if len(s.entries) > s.max {
		s.entries = s.entries[len(s.entries)-s.max:]
	}
}

func (s *Service) Format() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.entries) == 0 {
		return "Нет записей"
	}
	return "Недавно в стриме:\n" + strings.Join(s.entries, "\n")
}
