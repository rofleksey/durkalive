package database

import (
	"database/sql"
	"durkalive/app/config"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
	"github.com/samber/do"
)

//go:embed schema.sql
var schemaFS embed.FS

var _ do.Shutdownable = (*Service)(nil)

type Service struct {
	db *sql.DB
}

func New(di *do.Injector) (*Service, error) {
	cfg := do.MustInvoke[*config.Config](di)

	connStr := fmt.Sprintf("host=%s user=%s password=%s dbname=%s",
		cfg.DB.Host, cfg.DB.User, cfg.DB.Pass, cfg.DB.Database)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	schemaSQL, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to read schema: %w", err)
	}

	if _, err := db.Exec(string(schemaSQL)); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return &Service{db: db}, nil
}

func (s *Service) Shutdown() error {
	return s.db.Close()
}

func (s *Service) GetBotConfig() (*BotConfigRow, error) {
	var row BotConfigRow
	err := s.db.QueryRow(`SELECT id, data FROM bot_config WHERE id = 1`).Scan(
		&row.ID, &row.Data,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return &BotConfigRow{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get bot config: %w", err)
	}
	return &row, nil
}

func (s *Service) SaveBotConfig(data any) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal bot config: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO bot_config (id, data) VALUES (1, $1)
		ON CONFLICT (id) DO UPDATE SET data = $1
	`, string(jsonBytes))
	if err != nil {
		return fmt.Errorf("failed to save bot config: %w", err)
	}
	return nil
}

func (s *Service) AddFact(factText string, tags []string, usernames []string) (int64, error) {
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal tags: %w", err)
	}

	usernamesJSON, err := json.Marshal(usernames)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal usernames: %w", err)
	}

	var id int64
	err = s.db.QueryRow(`
		INSERT INTO facts (content, tags, usernames, created_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		RETURNING id
	`, factText, string(tagsJSON), string(usernamesJSON)).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to insert fact: %w", err)
	}

	return id, nil
}

func (s *Service) RemoveFact(factID int) error {
	_, err := s.db.Exec(`DELETE FROM facts WHERE id = $1`, factID)
	if err != nil {
		return fmt.Errorf("failed to delete fact %d: %w", factID, err)
	}
	return nil
}

func (s *Service) SearchFacts(requiredTags []string, anyTags []string, usernames []string, limit int) ([]Fact, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive")
	}

	query := `
        SELECT id, content, tags, usernames, created_at
        FROM facts
        WHERE 1=1
    `
	args := []any{}
	argPos := 1

	if len(usernames) > 0 {
		placeholders := make([]string, len(usernames))
		for i := range usernames {
			placeholders[i] = fmt.Sprintf("$%d", argPos)
			argPos++
		}
		query += ` AND EXISTS (
            SELECT 1 FROM jsonb_array_elements_text(facts.usernames::jsonb) AS elem
            WHERE elem IN (` + strings.Join(placeholders, ",") + `)
        )`
		for _, username := range usernames {
			args = append(args, username)
		}
	}

	if len(requiredTags) > 0 {
		for _, tag := range requiredTags {
			query += fmt.Sprintf(` AND EXISTS (
                SELECT 1 FROM jsonb_array_elements_text(facts.tags::jsonb) AS elem
                WHERE elem = $%d
            )`, argPos)
			args = append(args, tag)
			argPos++
		}
	}

	if len(anyTags) > 0 {
		anyPlaceholders := make([]string, len(anyTags))
		for i := range anyTags {
			anyPlaceholders[i] = fmt.Sprintf("$%d", argPos)
			argPos++
		}
		query += ` AND EXISTS (
            SELECT 1 FROM jsonb_array_elements_text(facts.tags::jsonb) AS elem
            WHERE elem IN (` + strings.Join(anyPlaceholders, ",") + `)
        )`
		for _, tag := range anyTags {
			args = append(args, tag)
		}
	}

	query += fmt.Sprintf(` ORDER BY RANDOM() LIMIT $%d`, argPos)
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search facts: %w", err)
	}
	defer rows.Close()

	var facts []Fact
	for rows.Next() {
		var f Fact
		var tagsJSON string
		var usernamesJSON string

		if err := rows.Scan(&f.ID, &f.Content, &tagsJSON, &usernamesJSON, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan fact: %w", err)
		}

		if err := json.Unmarshal([]byte(tagsJSON), &f.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}

		if err := json.Unmarshal([]byte(usernamesJSON), &f.Usernames); err != nil {
			return nil, fmt.Errorf("failed to unmarshal usernames: %w", err)
		}

		facts = append(facts, f)
	}

	return facts, nil
}
