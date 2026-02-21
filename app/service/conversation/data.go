package conversation

import (
	"sync"
	"time"
)

type DecisionResponse struct {
	NewSummary  string   `json:"new_summary"`
	AddFacts    []string `json:"add_facts"`
	RemoveFacts []int    `json:"remove_facts"`
	Confidence  float32  `json:"confidence"`
}

type ReplyResponse struct {
	Response string `json:"response"`
}

type State struct {
	mu sync.RWMutex

	summary       string
	chatHistory   ChatHistory
	lastReplyTime time.Time
}
