package conversation

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"durkalive/app/client/tts"
	"durkalive/app/config"
	"durkalive/app/service/memory"
	"durkalive/app/service/playback"
	"durkalive/app/service/recentmemory"

	"github.com/samber/do"
)

const answersDir = "data/answers"

const (
	maxReasonDuration = 10 * time.Second
	maxMessageLength  = 500
)

type Service struct {
	appCtx          context.Context
	cfg             *config.Config
	ttsClient       *tts.Client
	playbackSvc     *playback.Service
	memorySvc       *memory.Service
	recentMemorySvc *recentmemory.Service

	agent *Agent
	state *State
}

func New(di *do.Injector) (*Service, error) {
	cfg := do.MustInvoke[*config.Config](di)

	var state State
	recentMemorySvc := do.MustInvoke[*recentmemory.Service](di)
	memorySvc := do.MustInvoke[*memory.Service](di)
	agent := NewAgent(di, createClient(cfg.OpenAI.Agent), cfg.OpenAI.Agent.Model, &state, memorySvc, recentMemorySvc)

	s := &Service{
		appCtx:          do.MustInvoke[context.Context](di),
		cfg:             cfg,
		ttsClient:       do.MustInvoke[*tts.Client](di),
		playbackSvc:     do.MustInvoke[*playback.Service](di),
		memorySvc:       memorySvc,
		recentMemorySvc: recentMemorySvc,
		agent:           agent,
		state:           &state,
	}

	return s, nil
}

func (s *Service) ProcessMessage(ctx context.Context, username, text string) error {
	log := slog.With("username", username)
	ctx = WithLogger(ctx, log)
	totalStart := time.Now()

	defer func() {
		s.state.mu.Lock()
		s.state.chatHistory.add(username, text)
		s.state.mu.Unlock()
	}()

	s.state.mu.RLock()
	usernameMap := make(map[string]struct{})
	for _, msg := range s.state.chatHistory.messages {
		usernameMap[msg.Username] = struct{}{}
	}
	usernameMap[username] = struct{}{}
	s.state.mu.RUnlock()

	usernames := make([]string, 0, len(usernameMap))
	for u := range usernameMap {
		usernames = append(usernames, u)
	}

	stageStart := time.Now()
	result, err := s.agent.Call(ctx, username, text, usernames, s.cfg.Conversation.MaxSilenceSec)
	if err != nil {
		return fmt.Errorf("agent.Call: %w", err)
	}
	log.Debug("message processing stage", "stage", "agent", "duration", time.Since(stageStart), "duration_ms", time.Since(stageStart).Milliseconds())

	stageStart = time.Now()
	s.applyAgentResult(result)
	log.Debug("message processing stage", "stage", "apply_memory", "duration", time.Since(stageStart), "duration_ms", time.Since(stageStart).Milliseconds())

	if result.replyText == "" {
		log.Debug("message processing complete", "total_duration", time.Since(totalStart), "skipped", "no_reply")
		return nil
	}

	if len(result.replyText) > maxMessageLength {
		log.Warn("response too long, dropping", "len", len(result.replyText), "max", maxMessageLength)
		return nil
	}

	stageStart = time.Now()
	if err := s.sendMessage(ctx, result.replyText); err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	log.Debug("message processing stage", "stage", "send_message", "duration", time.Since(stageStart), "duration_ms", time.Since(stageStart).Milliseconds())

	s.state.mu.Lock()
	s.state.chatHistory.add(s.cfg.Bot.BotName, result.replyText)
	s.state.lastReplyTime = time.Now()
	s.state.mu.Unlock()

	if result.answerCtx != nil {
		if writeErr := s.writeAnswerDebug(result.answerCtx); writeErr != nil {
			LoggerFromContext(ctx).Warn("Failed to write answer debug file", "error", writeErr)
		}
	}

	return nil
}

func (s *Service) applyAgentResult(result *agentResult) {
	if result == nil {
		return
	}
	s.memorySvc.RemoveFacts(s.appCtx, result.removeIDs)
	for _, entry := range result.addRecent {
		s.recentMemorySvc.Add(entry)
	}
	for _, add := range result.addFacts {
		usernames := add.Usernames
		if len(usernames) == 0 {
			usernames = []string{s.cfg.Bot.UserName}
		}
		s.memorySvc.AddFact(s.appCtx, add.Content, add.Tags, usernames)
	}
}

func (s *Service) writeAnswerDebug(ctx *AnswerContext) error {
	if err := os.MkdirAll(answersDir, 0755); err != nil {
		return fmt.Errorf("mkdir answers: %w", err)
	}
	name := ctx.At.Format("2006-01-02_15-04-05") + ".txt"
	path := filepath.Join(answersDir, name)
	body := formatAnswerContext(ctx)
	return os.WriteFile(path, []byte(body), 0644)
}

func formatAnswerContext(ctx *AnswerContext) string {
	const sep = "==========\n"
	return sep + "At: " + ctx.At.Format(time.RFC3339) + "\n" +
		sep + "TRIGGER\nusername: " + ctx.TriggerUsername + "\nmessage: " + ctx.TriggerMessage + "\n" +
		sep + "PROMPT (SENT TO MODEL)\n" + ctx.Prompt + "\n" +
		sep + "REPLY\n" + ctx.Reply + "\n"
}

func (s *Service) sendMessage(ctx context.Context, text string) error {
	log := LoggerFromContext(ctx)

	wav, err := s.ttsClient.Synthesize(ctx, text)
	if err != nil {
		return fmt.Errorf("TTS synthesize: %w", err)
	}

	s.playbackSvc.BroadcastWAV(wav)
	log.Info("Replied with TTS", "text", text)
	return nil
}

func (s *Service) Close() error {
	return nil
}
