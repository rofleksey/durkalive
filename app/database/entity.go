package database

import (
	"time"
)

type BotConfigRow struct {
	ID   int    `json:"id"`
	Data string `json:"data"`
}

type Fact struct {
	ID        int       `json:"id"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags"`
	Usernames []string  `json:"usernames"`
	CreatedAt time.Time `json:"created_at"`
}

type SimilarFact struct {
	ID         int     `json:"id"`
	Content    string  `json:"content"`
	Similarity float32 `json:"similarity"`
}
