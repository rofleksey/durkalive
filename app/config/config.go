package config

import (
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Log          Log          `yaml:"log"`
	DB           DB           `yaml:"db"`
	Yandex       Yandex       `yaml:"yandex"`
	Audio        Audio        `yaml:"audio"`
	TTS          TTS          `yaml:"tts"`
	Bot          Bot          `yaml:"bot"`
	Webserver    Webserver    `yaml:"webserver"`
	OpenAI       OpenAI       `yaml:"openai"`
	Conversation Conversation `yaml:"conversation"`
}

// Audio holds capture device settings. MicDevice can contain spaces (e.g. "Microphone (USB Condenser Microphone)").
type Audio struct {
	MicDevice string `yaml:"mic_device"`
}

type TTS struct {
	APIKey  string `yaml:"api_key" validate:"required"`
	Speaker string `yaml:"speaker" validate:"required"`
	BaseURL string `yaml:"base_url"`
}

type Bot struct {
	BotName  string `yaml:"bot_name"`
	UserName string `yaml:"user_name"`
}

type Webserver struct {
	Listen string `yaml:"listen"`
}

type Conversation struct {
	MinReplyIntervalSec    int `yaml:"min_reply_interval_sec"`
	MaxSilenceSec          int `yaml:"max_silence_sec"`
	RecentMemoryMaxEntries int `yaml:"recent_memory_max_entries"`
}

type OpenAI struct {
	Agent     ModelConfig `yaml:"agent" validate:"required"`
	Embedding ModelConfig `yaml:"embedding" validate:"required"`
}

type ModelConfig struct {
	BaseURL string `yaml:"base_url" example:"https://openrouter.ai/api/v1" validate:"required"`
	Token   string `yaml:"token" example:"sk-proj-abc123456789DEF789ghi012JKL345mno678PQR901stu234VWX" validate:"required"`
	Model   string `yaml:"model" example:"deepseek/deepseek-chat-v3-0324:free" validate:"required"`
}

type Yandex struct {
	SpeechKit SpeechKit `yaml:"speech_kit"`
}

type SpeechKit struct {
}

type Log struct {
	Telegram TelegramLog `yaml:"telegram"`
}

type TelegramLog struct {
	Token  string `yaml:"token" example:"1234567890:ABCdefGHIjklMNopQRstUVwxyZ-123456789"`
	ChatID string `yaml:"chat_id" example:"1001234567890"`
}

type DB struct {
	User     string `yaml:"user" example:"postgres" validate:"required"`
	Pass     string `yaml:"pass" validate:"required"`
	Host     string `yaml:"host" example:"localhost:5432" validate:"required"`
	Database string `yaml:"database" example:"durkalive" validate:"required"`
}

func Load() (*Config, error) {
	var result Config

	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, oops.Errorf("failed to read config file: %w", err)
	}

	if err = yaml.Unmarshal(data, &result); err != nil {
		return nil, oops.Errorf("failed to parse YAML config: %w", err)
	}

	if result.DB.User == "" {
		result.DB.User = "postgres"
	}
	if result.DB.Pass == "" {
		result.DB.Pass = "postgres"
	}
	if result.DB.Host == "" {
		result.DB.Host = "localhost:5432"
	}
	if result.DB.Database == "" {
		result.DB.Database = "durkalive"
	}

	if result.Conversation.MinReplyIntervalSec == 0 {
		result.Conversation.MinReplyIntervalSec = 45
	}
	if result.Conversation.MaxSilenceSec == 0 {
		result.Conversation.MaxSilenceSec = 90
	}
	if result.Conversation.RecentMemoryMaxEntries == 0 {
		result.Conversation.RecentMemoryMaxEntries = 30
	}
	if result.TTS.BaseURL == "" {
		result.TTS.BaseURL = "https://ntts.fdev.team/api/v1/tts"
	}
	if result.Bot.BotName == "" {
		result.Bot.BotName = "assistant"
	}
	if result.Bot.UserName == "" {
		result.Bot.UserName = "user"
	}
	if result.Webserver.Listen == "" {
		result.Webserver.Listen = ":8080"
	}

	validate := validator.New(validator.WithRequiredStructEnabled())
	if err := validate.Struct(result); err != nil {
		return nil, oops.Errorf("failed to validate config: %w", err)
	}

	return &result, nil
}
