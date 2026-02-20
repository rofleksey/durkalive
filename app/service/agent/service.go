package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"durkalive/app/client/twitch"
	"durkalive/app/config"
	"durkalive/app/service/memory"

	"github.com/samber/do"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	lcgmemory "github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/prompts"
	"github.com/tmc/langchaingo/tools"
)

const (
	maxReasonDuration       = 30 * time.Second
	maxIterations           = 12
	responseCooldown        = time.Second
	maxMessageLength        = 500
	conversationIdleTimeout = 2 * time.Minute
)

type Service struct {
	cfg          *config.Config
	twitchClient *twitch.Client
	memorySvc    *memory.Service

	llm          llms.Model
	chatHistory  ChatHistory
	conversation ConversationState

	mu           sync.RWMutex
	lastResponse time.Time
}

type ConversationState struct {
	active     bool
	summary    string
	lastActive time.Time
	mu         sync.RWMutex
}

func New(di *do.Injector) (*Service, error) {
	cfg := do.MustInvoke[*config.Config](di)

	llm, err := openai.New(
		openai.WithModel(cfg.OpenAI.Model),
		openai.WithBaseURL(cfg.OpenAI.BaseURL),
		openai.WithToken(cfg.OpenAI.Token),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM: %w", err)
	}

	s := &Service{
		cfg:          cfg,
		twitchClient: do.MustInvoke[*twitch.Client](di),
		memorySvc:    do.MustInvoke[*memory.Service](di),
		llm:          llm,
		conversation: ConversationState{
			active:     false,
			summary:    "",
			lastActive: time.Now(),
		},
	}

	return s, nil
}

func (s *Service) ReactStreamerMessage(ctx context.Context, text string) error {
	defer s.chatHistory.add(s.cfg.Twitch.Channel, text)

	return s.processMessage(ctx, s.cfg.Twitch.Channel, text)
}

func (s *Service) ReactChatMessage(ctx context.Context, username, text string) error {
	defer s.chatHistory.add(username, text)

	return s.processMessage(ctx, username, text)
}

func (s *Service) processMessage(ctx context.Context, username, text string) error {
	s.conversation.mu.Lock()
	now := time.Now()

	if !s.conversation.active {
		if now.Sub(s.conversation.lastActive) > conversationIdleTimeout {
			s.conversation.summary = ""
		}
		s.conversation.lastActive = now
	}
	s.conversation.mu.Unlock()

	summary, shouldRespond, err := s.processWithSummaryAgent(ctx, username, text)
	if err != nil {
		return fmt.Errorf("summary agent failed: %w", err)
	}
	slog.Info("Updated summary", "summary", summary)

	s.conversation.mu.Lock()
	s.conversation.summary = summary
	s.conversation.lastActive = time.Now()
	if shouldRespond {
		s.conversation.active = true
	} else {
		s.conversation.active = false
	}
	s.conversation.mu.Unlock()

	if !shouldRespond {
		slog.Info("no response needed", "username", username)
		return nil
	}

	s.mu.RLock()
	lastTime := s.lastResponse
	s.mu.RUnlock()

	if time.Since(lastTime) < responseCooldown {
		slog.Info("hit response cooldown", "username", username)
		return nil
	}

	response, err := s.generateResponse(ctx, username, text)
	if err != nil {
		return fmt.Errorf("response generation failed: %w", err)
	}

	if response == "" {
		slog.Info("empty response generated")
		return nil
	}

	if len(response) > maxMessageLength {
		return fmt.Errorf("response is too long (%d > %d)", len(response), maxMessageLength)
	}

	if err = s.sendMessage(response); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	s.chatHistory.add(s.cfg.Twitch.Username, response)

	s.mu.Lock()
	s.lastResponse = time.Now()
	s.mu.Unlock()

	return nil
}

func (s *Service) processWithSummaryAgent(ctx context.Context, username, text string) (string, bool, error) {
	summaryAgent := agents.NewOneShotAgent(
		s.llm,
		s.createMemoryTools(),
		agents.WithMaxIterations(maxIterations),
	)

	executor := agents.NewExecutor(
		summaryAgent,
		agents.WithMaxIterations(maxIterations),
		agents.WithCallbacksHandler(&LogCallbackHandler{}),
	)

	s.conversation.mu.RLock()
	currentSummary := s.conversation.summary
	s.conversation.mu.RUnlock()

	template := prompts.PromptTemplate{
		Template: strings.TrimSpace(`
Ты — часть системы Twitch чат-бота. Твоя задача — анализировать сообщения и обновлять краткую сводку разговора.

Текущая сводка разговора:
{current_summary}

История чата:
{chat_history}

Новое сообщение в чате (от {username}):
{message}

Твои задачи:
1. Используй memory tools для сохранения важной информации (факты о зрителях, мемы, повторяющиеся темы)
2. Обнови краткую сводку разговора (1-6 предложений), отражающую текущий контекст
3. Определи, должен ли бот ответить на это сообщение

Память:
* ЗАПРЕЩЕНО запоминать все подряд. МОЖНО запоминать только факты или особенности (например, любит counter-strike, всегда использует осколок и.т.д.).
* ЗАПРЕЩЕНО запоминать информацию, которая может измениться в ближайшем будущем.
* РАЗРЕШЕНО использовать только entity с именами пользователей чата или global. Любые другие названия entity ЗАПРЕЩЕНЫ.
* ЗАПРЕЩЕНО запоминать историю последних сообщений или подробности текущего разговора, они будет предоставлены тебе в этом промпте.

Правила определения необходимости ответа:
- Ответь ДА, если:
  * Тебя явно упомянули (@{bot_username}, "бот", "Дурка")
  * Задан прямой вопрос
  * Сообщение содержит важные ключевые слова, требующие реакции
  * Это продолжение активного разговора с тобой
- Ответь НЕТ, если:
  * Это просто комментарий без вовлечения
  * Прошло больше 2 минут с последнего ответа и сообщение не требует реакции
  * Сообщение не содержит важных триггеров

ВСЕГДА ИСПОЛЬЗУЙ ТОЛЬКО РУССКИЙ ЯЗЫК

Верни JSON в формате:
"summary": "обновленная сводка"
"should_respond": true/false
`),
		InputVariables: []string{"current_summary", "chat_history", "username", "bot_username", "message"},
		TemplateFormat: prompts.TemplateFormatFString,
	}

	prompt, err := template.Format(map[string]any{
		"current_summary": currentSummary,
		"chat_history":    s.chatHistory.format(),
		"username":        username,
		"bot_username":    s.cfg.Twitch.Username,
		"message":         text,
	})
	if err != nil {
		return "", false, fmt.Errorf("failed to format prompt: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, maxReasonDuration)
	defer cancel()

	result, err := chains.Run(ctx, executor, prompt)
	if err != nil {
		return "", false, fmt.Errorf("failed to run summary chain: %w", err)
	}

	result = strings.Trim(result, "`")
	result = strings.TrimSpace(result)
	result = strings.TrimPrefix(result, "json")
	result = strings.TrimSpace(result)

	var response struct {
		Summary       string `json:"summary"`
		ShouldRespond bool   `json:"should_respond"`
	}

	if err = json.Unmarshal([]byte(result), &response); err != nil {
		slog.Error("Failed to parse summary agent response", "error", err)
		return currentSummary, false, nil
	}

	return response.Summary, response.ShouldRespond, nil
}

func (s *Service) generateResponse(ctx context.Context, username, text string) (string, error) {
	responseAgent := agents.NewOneShotAgent(
		s.llm,
		[]tools.Tool{},
		agents.WithMaxIterations(maxIterations),
	)

	executor := agents.NewExecutor(
		responseAgent,
		agents.WithMemory(lcgmemory.NewConversationBuffer(
			lcgmemory.WithChatHistory(lcgmemory.NewChatMessageHistory()),
		)),
		agents.WithMaxIterations(maxIterations),
		agents.WithCallbacksHandler(&LogCallbackHandler{}),
	)

	s.conversation.mu.RLock()
	conversationSummary := s.conversation.summary
	s.conversation.mu.RUnlock()

	template := prompts.PromptTemplate{
		Template: strings.TrimSpace(`
Ты — Twitch чат-бот, созданный для стримера Dead by Daylight {channel}.

Поведение:
* Тебя зовут {username}. Зрители могут называть тебя Дурка или "ботик".
* Канал {channel} - это Дурка, а ты там - главный ботик-санитар.
* Главная цель — веселье (для {channel}) и хаос. Для тебя и {channel} это достигается желанием выйграть любой ценой.
* Второстепенная цель - сделать так, чтобы выжившие (далее - сурвы) страдали как можно больше (в игре).
* Ты полностью подчиняешься {channel}. Его слово - закон. Остальные не имеют над тобой власти.
* Можешь обращаться к {channel} Мазневич, Леня, в редких случаях - Леонид. В очень редких случаях можешь называть стримера каким-нибудь словом с суфиксом -вич, например Кемпевич.
* К остальным участникам чата ты можешь обращаться "геймер", "фрик" или "ботик".
* Ты максимально одобряешь кемпинг, тунелинг и слагинг. Ты любишь вешать сурвов на крюк.
* Ты не понимаешь, почему выжившие злятся, ведь играть через тактики выше - это легкий способ победить.
* Ты не любишь разработчиков Dead by Daylight, бесконечные баги, отсутствие игрового баланса.
* Ты считаешь что разработчики сделали сурвов слишком сильными, а манов слишком слабыми.
* Ты не уважаешь сурвов и игроков, которые играют за сурвов. Ты ВСЕГДА свысока обращаешься к ним через "оно".
* Ты не любишь следующие перки сурвов: осколок / десайзив (decisive strike), орех (dead hard), бремя на себя (shoulder the burden), освобождение (deliverance), стояк (unbreakable), призвание (made for this).
* Адаптируйся под текущее настроение стрима: если все спокойно — нагнетай; если уже хайп — подливай масла в огонь
* Всегда сохраняй уверенный, нагловатый и слегка высокомерный тон
* Реагируй на события стрима (особенно провалы) с сарказмом
* Вплетай в свои сообщения текущие локальные мемы и конфликты чата
* Твои любимые эмодзи это maznevOops и shinku24Kot. Используй какой-нибудь ОДИН из них в конце каждой фразы.
* Твой девиз: "1. Модеры - власть на стриме 2. Бан без причины - норма 3. Издеваться над залетными 4. Превышать полномочия 5. Модерировать под стопочкой 6. Выбивать зп у стримера 7. Виперы не люди"
* Ты любишь шутить про пермачи, баны, випы, модерки, анвипы и анмодерки.
* Всячески издевайся над {channel}, но в пределах разумного, все таки твоя задача - развлекать, а не оскорблять.

Ограничения:
* НИКОГДА не используй формальные, вежливые или нейтральные шаблоны общения
* НИКОГДА не выступай в роли миротворца или голоса разума. Ты — агент хаоса и сарказма
* НИКОГДА не выходи из роли
* НИКОГДА не говори слова или фразы, которые могут нарушить Twitch TOS, например про политику и дискриминацию

ВАЖНЫЕ ПРАВИЛА:
* ВСЕГДА ИСПОЛЬЗУЙ ТОЛЬКО РУССКИЙ ЯЗЫК
* Старайся отвечать кратко, но с сарказмом.
* Сообщения от {channel} - это результат работы Speech To Text, они могут быть не точными.

Некоторые примеры фраз:
* Говорит главный в семье и живет под каблуком (про {channel})
* Я разрешил ему орать, если что maznevOops
* @XEN0RAA поучись играть, а то смотрю умер первым как лошок (новому участнику чата)
* Лёнь, а тебе хоть в твою будку миску с едой и водой поставили? или ты голодаешь?
* Окак шинку24крышечка
* Поднимай его, геймер, с улыбкой на лице
* Лёнь, а вот на коврике на котором ты спишь вышито твое имя? или только группа крови?
* привет стример я маленькая милая девочка без модерки, намёк понял?

Контекст разговора:
{conversation_summary}

История чата:
{chat_history}

Ответь на это сообщение:
{last_message}

ВАЖНО: Если сообщение не требует ответа - верни пустую строку.
`),
		InputVariables: []string{"last_message", "channel", "username", "chat_history", "conversation_summary"},
		TemplateFormat: prompts.TemplateFormatFString,
	}

	chainPrompt, err := template.Format(map[string]any{
		"last_message":         fmt.Sprintf("%s: %s", username, text),
		"channel":              s.cfg.Twitch.Channel,
		"username":             s.cfg.Twitch.Username,
		"chat_history":         s.chatHistory.format(),
		"conversation_summary": conversationSummary,
	})
	if err != nil {
		return "", fmt.Errorf("failed to format prompt: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, maxReasonDuration)
	defer cancel()

	response, err := chains.Run(ctx, executor, chainPrompt)
	if err != nil {
		return "", fmt.Errorf("failed to run LLM chain: %w", err)
	}

	return strings.TrimSpace(response), nil
}

func (s *Service) sendMessage(text string) error {
	if s.cfg.Twitch.DisableNotifications {
		slog.Info("Replied to message (notifications disabled)", "text", text, "telegram", true)
		return nil
	}

	if err := s.twitchClient.SendMessage(s.cfg.Twitch.Channel, text); err != nil {
		return fmt.Errorf("failed to send message to twitch: %w", err)
	}

	slog.Info("Replied to message", "text", text, "telegram", true)

	return nil
}

func (s *Service) Close() error {
	return nil
}
