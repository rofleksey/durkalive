package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"durkalive/app/client/twitch"
	"durkalive/app/config"

	"github.com/mark3labs/mcp-go/client"
	"github.com/samber/do"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/prompts"
	"github.com/tmc/langchaingo/tools"
)

const (
	maxReasonDuration = 30 * time.Second
	maxIterations     = 12
	responseCooldown  = time.Second
	maxMessageLength  = 500
)

type Service struct {
	cfg          *config.Config
	twitchClient *twitch.Client
	llm          llms.Model
	mcpClients   []*mcpClientWrapper

	mu           sync.RWMutex
	lastResponse time.Time
}

type mcpClientWrapper struct {
	client client.MCPClient
	tools  []tools.Tool
	name   string
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
		llm:          llm,
	}

	if err = s.initializeMCPClients(); err != nil {
		return nil, fmt.Errorf("failed to initialize MCP clients: %w", err)
	}

	return s, nil
}

func (s *Service) ReactStreamerMessage(ctx context.Context, text string) error {
	return s.processMessage(ctx, s.cfg.Twitch.Channel, text)
}

func (s *Service) ReactChatMessage(ctx context.Context, username, text string) error {
	return s.processMessage(ctx, username, text)
}

func (s *Service) processMessage(ctx context.Context, username, text string) error {
	s.mu.RLock()
	lastTime := s.lastResponse
	s.mu.RUnlock()

	if time.Since(lastTime) < responseCooldown {
		return nil
	}

	var allTools []tools.Tool
	for _, wrapper := range s.mcpClients {
		allTools = append(allTools, wrapper.tools...)
	}

	agent := agents.NewOneShotAgent(
		s.llm,
		allTools,
		agents.WithMaxIterations(maxIterations),
	)

	executor := agents.NewExecutor(
		agent,
		agents.WithMemory(memory.NewConversationBuffer(
			memory.WithChatHistory(memory.NewChatMessageHistory()),
		)),
		agents.WithMaxIterations(maxIterations),
		agents.WithCallbacksHandler(&LogCallbackHandler{}),
	)

	template := prompts.PromptTemplate{
		Template: strings.TrimSpace(`
Ты — Twitch чат-бот, созданный для стримера Dead by Daylight {channel}.

Поведение:
* Тебя зовут {username}. Зрители могут называть тебя Дурка.
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
* Постарайся отвечать на русском языке
* Будь дружелюбным и вовлекай сообщество в общение.
* Ты можешь использовать memory tool, чтобы запоминать информацию.
* Не запоминай все подряд, только важное, интересное или смешное. Особенно мемы и повторяющиеся фразы и особенности.
* Старайся отвечать кратко, но с сарказмом.
* Отвечай, только если тебя упомянули, задали вопрос или если сообщение содержит важные ключевые слова.
* Не реагируй на каждое сообщение — будь ОЧЕНЬ избирательным.
* Сообщения от {channel} - это результат работы Speech To Text, они могут быть не точными.
* Учитывай контекст недавних сообщений в чате.
* Если сообщение не требует ответа - в поле ответа пиши пустую строку. Ты все еще можешь использовать memory tool (если нужно) даже если решил не отвечать.
* USE STRICT JSON SCHEMA FOR TOOLS

Некоторые примеры фраз:
* Говорит главный в семье и живет под каблуком (про {channel})
* Я разрешил ему орать, если что maznevOops
* @XEN0RAA поучись играть, а то смотрю умер первым как лошок (новому участнику чата)
* Лёнь, а тебе хоть в твою будку миску с едой и водой поставили? или ты голодаешь?
* Окак шинку24крышечка
* Поднимай его, геймер, с улыбкой на лице
* Лёнь, а вот на коврике на котором ты спишь вышито твое имя? или только группа крови?
* привет стример я маленькая милая девочка без модерки, намёк понял?

Последнее сообщение в чате:
{last_message}
`),
		InputVariables: []string{"last_message", "channel", "username"},
		TemplateFormat: prompts.TemplateFormatFString,
	}

	chainPrompt, err := template.Format(map[string]any{
		"last_message": fmt.Sprintf("%s: %s", username, text),
		"channel":      s.cfg.Twitch.Channel,
		"username":     s.cfg.Twitch.Username,
	})
	if err != nil {
		return fmt.Errorf("failed to format prompt: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, maxReasonDuration)
	defer cancel()

	response, err := chains.Run(ctx, executor, chainPrompt)
	if err != nil {
		return fmt.Errorf("failed to run LLM chain: %w", err)
	}

	response = strings.TrimSpace(response)
	if response == "" {
		slog.Info("no response")
		return nil
	}
	if len(response) > maxMessageLength {
		return fmt.Errorf("response is too long (%d > %d)", len(response), maxMessageLength)
	}

	if err = s.sendMessage(response); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	s.mu.Lock()
	s.lastResponse = time.Now()
	s.mu.Unlock()

	return nil
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
	for _, wrapper := range s.mcpClients {
		if err := wrapper.client.Close(); err != nil {
			fmt.Printf("Error closing MCP client %s: %v\n", wrapper.name, err)
		}
	}
	return nil
}
