package mylog

import (
	"context"
	"durkalive/app/config"
	"log/slog"
	"os"

	"github.com/phsym/console-slog"
	slogmulti "github.com/samber/slog-multi"
	slogtelegram "github.com/samber/slog-telegram/v2"
)

func Preinit() {
	slog.SetDefault(slog.New(console.NewHandler(os.Stderr, &console.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	})))
}

func Init(cfg *config.Config) error {
	router := slogmulti.Router()

	router = router.Add(console.NewHandler(os.Stderr, &console.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))

	if cfg.Log.Telegram.Token != "" {
		router = router.Add(
			slogtelegram.Option{
				Level:     slog.LevelDebug,
				Token:     cfg.Log.Telegram.Token,
				Username:  cfg.Log.Telegram.ChatID,
				AddSource: true,
			}.NewTelegramHandler(),

			func(_ context.Context, r slog.Record) bool {
				hasTelegram := false

				r.Attrs(func(attr slog.Attr) bool {
					if attr.Key == "telegram" {
						hasTelegram = true
						return false
					}

					return true
				})

				return r.Level == slog.LevelError || hasTelegram
			},
		)
	}

	slog.SetDefault(slog.New(router.Handler()))

	return nil
}
