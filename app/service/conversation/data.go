package conversation

import (
	"sync"
	"time"
)

type AnswerContext struct {
	At              time.Time
	TriggerUsername string
	TriggerMessage  string
	Prompt          string
	Reply           string
}

type State struct {
	mu sync.RWMutex

	chatHistory   ChatHistory
	lastReplyTime time.Time
}
