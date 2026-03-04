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

type DecisionResponse struct {
	AddFacts     []AddFactRequest `json:"add_facts"`
	AddRecent    []string         `json:"add_recent"`
	RemoveFacts  []int            `json:"remove_facts"`
	NeedResponse bool             `json:"need_response"`
}

type AddFactRequest struct {
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	Usernames []string `json:"usernames"`
}

type State struct {
	mu sync.RWMutex

	chatHistory   ChatHistory
	lastReplyTime time.Time
}
