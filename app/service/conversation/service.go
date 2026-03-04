// app/service/conversation/service.go
package conversation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"durkalive/app/client/twitch"
	"durkalive/app/config"
	"durkalive/app/service/memory"
	"durkalive/app/service/recentmemory"

	_ "embed"

	"github.com/samber/do"
)

const (
	maxReasonDuration = 30 * time.Second
	maxMessageLength  = 500
)

type Service struct {
	appCtx          context.Context
	cfg             *config.Config
	twitchClient    *twitch.Client
	memorySvc       *memory.Service
	recentMemorySvc *recentmemory.Service

	taggerAgent   *TaggerAgent
	decisionAgent *DecisionAgent
	replyAgent    *ReplyAgent
	reviewAgent   *ReviewAgent
	state         *State
}

func New(di *do.Injector) (*Service, error) {
	cfg := do.MustInvoke[*config.Config](di)

	var state State

	recentMemorySvc := do.MustInvoke[*recentmemory.Service](di)
	taggerAgent := NewTaggerAgent(di, createClient(cfg.OpenAI.Tagger), cfg.OpenAI.Tagger.Model,
		&state)
	decisionAgent := NewDecisionAgent(di, createClient(cfg.OpenAI.Decision), cfg.OpenAI.Decision.Model, &state)
	replyAgent := NewReplyAgent(di, createClient(cfg.OpenAI.Reply), cfg.OpenAI.Reply.Model, &state, recentMemorySvc)
	reviewAgent := NewReviewAgent(di, createClient(cfg.OpenAI.Review), cfg.OpenAI.Review.Model)

	s := &Service{
		appCtx:          do.MustInvoke[context.Context](di),
		cfg:             cfg,
		twitchClient:    do.MustInvoke[*twitch.Client](di),
		memorySvc:       do.MustInvoke[*memory.Service](di),
		recentMemorySvc: recentMemorySvc,
		taggerAgent:     taggerAgent,
		decisionAgent:   decisionAgent,
		replyAgent:      replyAgent,
		reviewAgent:     reviewAgent,
		state:           &state,
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

	go s.applyMemoryChanges(result)

	needReply := result.NeedResponse
	if !needReply {
		s.state.mu.RLock()
		lastReply := s.state.lastReplyTime
		s.state.mu.RUnlock()
		maxSilence := time.Duration(s.cfg.Conversation.MaxSilenceSec) * time.Second
		if !lastReply.IsZero() && time.Since(lastReply) > maxSilence {
			needReply = true
		}
	}
	if !needReply {
		return nil
	}

	minGap := time.Duration(s.cfg.Conversation.MinReplyIntervalSec) * time.Second
	s.state.mu.RLock()
	lastReply := s.state.lastReplyTime
	s.state.mu.RUnlock()
	if !lastReply.IsZero() && time.Since(lastReply) < minGap {
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
	s.memorySvc.RemoveFacts(s.appCtx, result.RemoveFacts)
	for _, entry := range result.AddRecent {
		s.recentMemorySvc.Add(entry)
	}
	for _, addReq := range result.AddFacts {
		if len(addReq.Usernames) == 0 {
			addReq.Usernames = []string{s.cfg.Twitch.Channel}
		}

		s.memorySvc.AddFact(s.appCtx, addReq.Content, addReq.Tags, addReq.Usernames)
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

	s.state.mu.RLock()
	contextStr := s.state.chatHistory.format()
	s.state.mu.RUnlock()
	lastMsg := fmt.Sprintf("%s: %s", username, text)
	ok, reviewErr := s.reviewAgent.Approve(ctx, lastMsg, replyText, contextStr)
	if reviewErr != nil {
		return fmt.Errorf("reviewAgent.Approve: %w", reviewErr)
	}
	if !ok {
		return nil
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
