package conversation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"durkalive/app/client/twitch"
	"durkalive/app/config"
	"durkalive/app/service/memory"

	_ "embed"

	"github.com/samber/do"
)

const (
	minConfidence     = 0.8
	maxReasonDuration = 30 * time.Second
	maxMessageLength  = 500
)

type Service struct {
	cfg          *config.Config
	twitchClient *twitch.Client
	memorySvc    *memory.Service

	taggerAgent   *TaggerAgent
	decisionAgent *DecisionAgent
	replyAgent    *ReplyAgent
	state         *State
}

func New(di *do.Injector) (*Service, error) {
	cfg := do.MustInvoke[*config.Config](di)
	memorySvc := do.MustInvoke[*memory.Service](di)

	var state State

	taggerAgent := NewTaggerAgent(cfg, createClient(cfg.OpenAI.Tagger), cfg.OpenAI.Tagger.Model,
		&state)
	decisionAgent := NewDecisionAgent(cfg, memorySvc, createClient(cfg.OpenAI.Decision), cfg.OpenAI.Decision.Model,
		&state)
	replyAgent := NewReplyAgent(cfg, memorySvc, createClient(cfg.OpenAI.Reply), cfg.OpenAI.Reply.Model, &state)

	s := &Service{
		cfg:           cfg,
		twitchClient:  do.MustInvoke[*twitch.Client](di),
		memorySvc:     memorySvc,
		taggerAgent:   taggerAgent,
		decisionAgent: decisionAgent,
		replyAgent:    replyAgent,
		state:         &state,
	}

	return s, nil
}

func (s *Service) ProcessMessage(ctx context.Context, username, text string) error {
	defer func() {
		s.state.mu.Lock()
		s.state.chatHistory.add(username, text)
		s.state.mu.Unlock()
	}()

	tags, err := s.taggerAgent.Call(ctx, username, text)
	if err != nil {
		return fmt.Errorf("taggerAgent.Call: %w", err)
	}

	s.state.mu.RLock()
	usernameMap := make(map[string]struct{})
	for _, msg := range s.state.chatHistory.messages {
		usernameMap[msg.Username] = struct{}{}
	}
	usernameMap[username] = struct{}{}
	s.state.mu.RUnlock()

	usernames := make([]string, 0, len(usernameMap))
	for curUsername := range usernameMap {
		usernames = append(usernames, curUsername)
	}

	result, err := s.decisionAgent.Call(ctx, username, text, tags, usernames)
	if err != nil {
		return fmt.Errorf("decisionAgent.Call: %w", err)
	}

	for i, value := range result.RemoveFacts {
		result.RemoveFacts[i] = value - 1
	}

	s.applyMemoryChanges(result)

	if !result.NeedResponse {
		slog.Debug("Response is not required")
		return nil
	}

	go func() {
		if err := s.generateReply(ctx, username, text, tags, usernames); err != nil {
			slog.Error("Failed to generate reply",
				"username", username,
				"text", text,
				"error", err,
			)
		}
	}()

	return nil
}

func (s *Service) applyMemoryChanges(result *DecisionResponse) {
	s.memorySvc.RemoveFacts(result.RemoveFacts)

	for _, addReq := range result.AddFacts {
		if len(addReq.Usernames) == 0 {
			addReq.Usernames = []string{s.cfg.Twitch.Channel}
		}

		s.memorySvc.AddFact(addReq.Content, addReq.Tags, addReq.Usernames)
	}
}

func (s *Service) generateReply(ctx context.Context, username, text string, tags, usernames []string) error {
	replyText, err := s.replyAgent.Call(ctx, username, text, tags, usernames)
	if err != nil {
		return fmt.Errorf("replyAgent.Call: %w", err)
	}

	if len(replyText) > maxMessageLength {
		return fmt.Errorf("response is too long (%d > %d)", len(replyText), maxMessageLength)
	}

	if err = s.sendMessage(replyText); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	s.state.mu.Lock()
	s.state.chatHistory.add(s.cfg.Twitch.Username, replyText)
	s.state.lastReplyTime = time.Now()
	s.state.mu.Unlock()

	return nil
}

func (s *Service) sendMessage(text string) error {
	if s.cfg.Twitch.DisableNotifications {
		slog.Info("Replied to message (notifications disabled)",
			"text", text,
			"telegram", true)
		return nil
	}

	if err := s.twitchClient.SendMessage(s.cfg.Twitch.Channel, text); err != nil {
		return fmt.Errorf("failed to send message to twitch: %w", err)
	}

	slog.Info("Replied to message",
		"text", text,
		"telegram", true)

	return nil
}

func (s *Service) Close() error {
	return nil
}
