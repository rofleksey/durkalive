package speechkit

import (
	"context"
	"fmt"
	"strings"

	"github.com/yandex-cloud/go-genproto/yandex/cloud/ai/stt/v3"
)

type Handle struct {
	client stt.Recognizer_RecognizeStreamingClient
	cancel context.CancelFunc
}

func (h *Handle) Send(content []byte) error {
	var req stt.StreamingRequest
	req.SetChunk(&stt.AudioChunk{
		Data: content,
	})

	return h.client.Send(&req)
}

func (h *Handle) SendConfig() error {
	var audioFormatOpts stt.AudioFormatOptions
	audioFormatOpts.SetRawAudio(&stt.RawAudio{
		AudioEncoding:     stt.RawAudio_LINEAR16_PCM,
		SampleRateHertz:   16000,
		AudioChannelCount: 1,
	})

	var eouClassifier stt.EouClassifierOptions
	eouClassifier.SetDefaultClassifier(&stt.DefaultEouClassifier{
		Type:                       stt.DefaultEouClassifier_HIGH,
		MaxPauseBetweenWordsHintMs: 500,
	})

	var req stt.StreamingRequest
	req.SetSessionOptions(&stt.StreamingOptions{
		RecognitionModel: &stt.RecognitionModelOptions{
			Model:       "general",
			AudioFormat: &audioFormatOpts,
			LanguageRestriction: &stt.LanguageRestrictionOptions{
				RestrictionType: stt.LanguageRestrictionOptions_WHITELIST,
				LanguageCode:    []string{"ru-RU"},
			},
		},
		EouClassifier: &eouClassifier,
	})

	return h.client.Send(&req)
}

func (h *Handle) Recv() ([]string, error) {
	res, err := h.client.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to receive stt: %w", err)
	}

	finalEvent := res.GetFinal()
	if finalEvent == nil {
		return nil, nil
	}

	result := make([]string, 0, len(finalEvent.Alternatives))
	for _, alt := range finalEvent.Alternatives {
		text := strings.TrimSpace(alt.Text)
		if text == "" {
			continue
		}

		result = append(result, text)
	}

	return result, nil
}

func (h *Handle) Close() error {
	h.cancel()
	return nil
}
