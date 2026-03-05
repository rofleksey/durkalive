package conversation

import (
	"context"
	"durkalive/app/config"
	"durkalive/app/database"
	"durkalive/app/service/embedding"
	"durkalive/app/service/memory"
	"durkalive/app/service/recentmemory"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	_ "embed"

	"github.com/samber/do"
	"github.com/sashabaranov/go-openai"
)

//go:embed agent_prompt_template.txt
var agentPromptTemplate string

func agentToolDefinitions() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "add_fact",
				Description: "Add a fact to long-term memory. Use for permanent facts about the user (e.g. likes RPG, can dance). Content in Russian.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content":   map[string]any{"type": "string", "description": "Fact text in Russian"},
						"tags":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags in English, one word each"},
						"usernames": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Usernames this fact relates to"},
					},
					"required": []string{"content", "usernames"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "add_recent",
				Description: "Add short notes for this session only (recent memory).",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"entries": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Short session notes"},
					},
					"required": []string{"entries"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "remove_facts",
				Description: "Remove facts from long-term memory by their ids (from similar_facts list).",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "Fact ids to remove"},
					},
					"required": []string{"ids"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "reply",
				Description: "Send a reply message to the chat. Use when you want to respond. At most one reply per turn.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"text": map[string]any{"type": "string", "description": "Reply text (short, Russian, lowercase)"},
					},
					"required": []string{"text"},
				},
			},
		},
	}
}

type Agent struct {
	cfg             *config.Config
	db              *database.Service
	embeddingSvc    *embedding.Service
	memorySvc       *memory.Service
	recentMemorySvc *recentmemory.Service

	client *openai.Client
	model  string

	state *State
}

func NewAgent(
	di *do.Injector,
	client *openai.Client,
	model string,
	state *State,
	memorySvc *memory.Service,
	recentMemorySvc *recentmemory.Service,
) *Agent {
	return &Agent{
		cfg:             do.MustInvoke[*config.Config](di),
		db:              do.MustInvoke[*database.Service](di),
		embeddingSvc:    do.MustInvoke[*embedding.Service](di),
		memorySvc:       memorySvc,
		recentMemorySvc: recentMemorySvc,
		client:          client,
		model:           model,
		state:           state,
	}
}

type agentResult struct {
	replyText string
	answerCtx *AnswerContext
	addFacts  []addFactArgs
	addRecent []string
	removeIDs []int
}

type addFactArgs struct {
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	Usernames []string `json:"usernames"`
}

func (a *Agent) Call(ctx context.Context, username, text string, usernames []string, maxSilenceSec int) (*agentResult, error) {
	start := time.Now()
	defer func() {
		slog.Debug("Call took", "time", time.Since(start))
	}()

	a.state.mu.RLock()
	lastReplyTime := a.state.lastReplyTime
	similarFactsStr, err := formatFactsByChatHistory(ctx, a.db, a.embeddingSvc, a.state, usernames)
	if err != nil {
		a.state.mu.RUnlock()
		return nil, fmt.Errorf("format similar facts: %w", err)
	}
	historyStr := a.state.chatHistory.format()
	a.state.mu.RUnlock()

	now := time.Now()
	var lastReply string
	if lastReplyTime.IsZero() {
		lastReply = "Ты еще не писал сообщений в чат"
	} else {
		lastReply = fmt.Sprintf("Ты отвечал %d секунд назад", int(now.Sub(lastReplyTime).Seconds()))
	}

	silenceHint := ""
	if maxSilenceSec > 0 && !lastReplyTime.IsZero() && time.Since(lastReplyTime) > time.Duration(maxSilenceSec)*time.Second {
		silenceHint = "* Прошло много времени с твоего последнего ответа — желательно ответить."
	}

	recentMemoryStr := "Нет записей"
	if a.recentMemorySvc != nil {
		recentMemoryStr = a.recentMemorySvc.Format()
	}

	templateValues := map[string]any{
		"last_message":  fmt.Sprintf("%s - %s: %s", formatTime(now), username, text),
		"last_reply":    lastReply,
		"silence_hint":  silenceHint,
		"channel":       a.cfg.Bot.UserName,
		"username":      a.cfg.Bot.BotName,
		"chat_history":  historyStr,
		"recent_memory": recentMemoryStr,
		"similar_facts": similarFactsStr,
	}

	prompt := agentPromptTemplate
	for key, value := range templateValues {
		prompt = strings.ReplaceAll(prompt, "{"+key+"}", fmt.Sprint(value))
	}

	ctx, cancel := context.WithTimeout(ctx, maxReasonDuration)
	defer cancel()

	req := openai.ChatCompletionRequest{
		Model: a.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: prompt},
			{Role: openai.ChatMessageRoleUser, Content: templateValues["last_message"].(string)},
		},
		Tools:               agentToolDefinitions(),
		ToolChoice:          "auto",
		MaxCompletionTokens: 1500,
		Temperature:         1,
	}

	resp, err := a.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no completion choice")
	}

	msg := resp.Choices[0].Message
	result := &agentResult{}

	for _, tc := range msg.ToolCalls {
		if tc.Type != openai.ToolTypeFunction || tc.Function.Arguments == "" {
			continue
		}
		name := tc.Function.Name
		args := tc.Function.Arguments

		switch name {
		case "add_fact":
			var p addFactArgs
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				continue
			}
			if p.Content != "" && len(p.Usernames) > 0 {
				result.addFacts = append(result.addFacts, p)
			}
		case "add_recent":
			var p struct {
				Entries []string `json:"entries"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				continue
			}
			result.addRecent = append(result.addRecent, p.Entries...)
		case "remove_facts":
			var p struct {
				IDs []int `json:"ids"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				continue
			}
			result.removeIDs = append(result.removeIDs, p.IDs...)
		case "reply":
			var p struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				continue
			}
			replyText := strings.TrimSpace(p.Text)
			if replyText != "" && result.replyText == "" {
				result.replyText = replyText
				result.answerCtx = &AnswerContext{
					At:              now,
					TriggerUsername: username,
					TriggerMessage:  text,
					Prompt:          prompt,
					Reply:           replyText,
				}
			}
		}
	}

	return result, nil
}
