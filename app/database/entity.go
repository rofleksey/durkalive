package database

import "time"

type BotConfigRow struct {
	ID   int    `json:"id"`
	Data string `json:"data"`
}

type Fact struct {
	ID        int       `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	Tags      []string  `json:"tags"`
}
