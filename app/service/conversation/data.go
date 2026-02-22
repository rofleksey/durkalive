package conversation

import (
	"sync"
	"time"
)

type DecisionResponse struct {
	AddFacts        []AddFactRequest         `json:"add_facts"`
	RemoveFacts     []int                    `json:"remove_facts"`
	UpdateRelevance []UpdateRelevanceRequest `json:"update_relevance"`
	NeedResponse    bool                     `json:"need_response"`
}

type AddFactRequest struct {
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	Relevance int      `json:"relevance"`
}

type UpdateRelevanceRequest struct {
	ID        int `json:"id"`
	Relevance int `json:"relevance"`
}

type State struct {
	mu sync.RWMutex

	chatHistory   ChatHistory
	lastReplyTime time.Time
}
