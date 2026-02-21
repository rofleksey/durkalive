package queue

import (
	"log/slog"

	"github.com/samber/do"
)

const bufferSize = 64

var _ do.Shutdownable = (*Service)(nil)

type Service struct {
	queue chan Message
}

type Message struct {
	Username string
	Text     string
}

func New(_ *do.Injector) (*Service, error) {
	return &Service{
		queue: make(chan Message, bufferSize),
	}, nil
}

func (s *Service) Add(username, text string) {
	defer func() {
		if r := recover(); r != nil {

		}
	}()

	select {
	case s.queue <- Message{username, text}:
	default:
		slog.Warn("message queue is full")
	}
}

func (s *Service) Channel() <-chan Message {
	return s.queue
}

func (s *Service) Shutdown() error {
	close(s.queue)

	return nil
}
