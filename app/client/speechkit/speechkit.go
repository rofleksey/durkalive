package speechkit

import (
	"context"
	"durkalive/app/config"
	"encoding/json"
	"fmt"
	"os"

	"github.com/samber/do"
	ycsdk "github.com/yandex-cloud/go-sdk"
	"github.com/yandex-cloud/go-sdk/iamkey"
)

type YandexSpeechKit struct {
	cfg *config.Config
	sdk *ycsdk.SDK
}

func NewClient(di *do.Injector) (*YandexSpeechKit, error) {
	ctx := do.MustInvoke[context.Context](di)
	cfg := do.MustInvoke[*config.Config](di)

	keyBytes, err := os.ReadFile("service-account-key.json")
	if err != nil {
		return nil, fmt.Errorf("could not read service account key: %w", err)
	}

	var key iamkey.Key
	if err = json.Unmarshal(keyBytes, &key); err != nil {
		return nil, fmt.Errorf("could not parse service account key: %w", err)
	}

	creds, err := ycsdk.ServiceAccountKey(&key)
	if err != nil {
		return nil, fmt.Errorf("could not create service account key: %w", err)
	}

	sdk, err := ycsdk.Build(ctx, ycsdk.Config{
		Credentials: creds,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Yandex SDK: %w", err)
	}

	return &YandexSpeechKit{
		cfg: cfg,
		sdk: sdk,
	}, nil
}

func (y *YandexSpeechKit) Start(ctx context.Context) (*Handle, error) {
	ctx, cancel := context.WithCancel(ctx)

	client, err := y.sdk.AI().STTV3().Recognizer().RecognizeStreaming(ctx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &Handle{
		client: client,
		cancel: cancel,
	}, nil
}
