package conversation

import (
	"sync"
	"time"
)

type DecisionResponse struct {
	AddFacts     []AddFactRequest `json:"add_facts"`
	RemoveFacts  []int            `json:"remove_facts"`
	NeedResponse bool             `json:"need_response"`
}

type AddFactRequest struct {
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

type State struct {
	mu sync.RWMutex

	chatHistory   ChatHistory
	lastReplyTime time.Time
}
