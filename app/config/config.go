package config

import (
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Log    Log    `yaml:"log"`
	DB     DB     `yaml:"db"`
	Yandex Yandex `yaml:"yandex"`
	Twitch Twitch `yaml:"twitch"`
	OpenAI OpenAI `yaml:"openai"`
}

type OpenAI struct {
	Decision ModelConfig `yaml:"decision" validate:"required"`
	Reply    ModelConfig `yaml:"reply" validate:"required"`
}

type ModelConfig struct {
	// OpenAI base url
	BaseURL string `yaml:"base_url" example:"https://openrouter.ai/api/v1" validate:"required"`
	// OpenAI token
	Token string `yaml:"token" example:"sk-proj-abc123456789DEF789ghi012JKL345mno678PQR901stu234VWX" validate:"required"`
	// OpenAI model
	Model string `yaml:"model" example:"deepseek/deepseek-chat-v3-0324:free" validate:"required"`
}

type Yandex struct {
	SpeechKit SpeechKit `yaml:"speech_kit"`
}

type SpeechKit struct {
}

type Twitch struct {
	// ClientID of the twitch application
	ClientID string `yaml:"client_id" example:"a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p" validate:"required"`
	// Client secret of the twitch application
	ClientSecret string `yaml:"client_secret" example:"abc123def456ghi789jkl012mno345pqr678stu901" validate:"required"`
	// Username of the bot account
	Username string `yaml:"username" example:"PogChamp123" validate:"required"`
	// Channel name of the channel
	Channel string `yaml:"channel" example:"PogChamp123" validate:"required"`
	// User refresh token of the bot account
	RefreshToken string `yaml:"refresh_token" example:"v1.abc123def456ghi789jkl012mno345pqr678stu901vwx234yz567" validate:"required"`
	// Disable notifications
	DisableNotifications bool `yaml:"disable_notifications" example:"false"`
	// Ignore chat
	IgnoreChat bool `yaml:"ignore_chat" example:"false"`
}

type Log struct {
	// Telegram logging config
	Telegram TelegramLog `yaml:"telegram"`
}

type TelegramLog struct {
	// Chat bot token, obtain it via BotFather
	Token string `yaml:"token" example:"1234567890:ABCdefGHIjklMNopQRstUVwxyZ-123456789"`
	// Chat ID to send messages to
	ChatID string `yaml:"chat_id" example:"1001234567890"`
}

type DB struct {
	// Postgres username
	User string `yaml:"user" example:"postgres" validate:"required"`
	// Postgres password
	Pass string `yaml:"pass" validate:"required"`
	// Postgres host
	Host string `yaml:"host"  example:"localhost:5432" validate:"required"`
	// Postgres database name
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

	validate := validator.New(validator.WithRequiredStructEnabled())
	if err := validate.Struct(result); err != nil {
		return nil, oops.Errorf("failed to validate config: %w", err)
	}

	return &result, nil
}
