package database

import (
	"database/sql"
	"embed"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/samber/do"
)

//go:embed schema.sql
var schemaFS embed.FS

var _ do.Shutdownable = (*Service)(nil)

type Service struct {
	db *sql.DB
}

func New(_ *do.Injector) (*Service, error) {
	db, err := sql.Open("sqlite", "data/database.db?cache=shared&_foreign_keys=1")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
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
		INSERT INTO bot_config (id, data) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET data = ?
	`, string(jsonBytes))
	if err != nil {
		return fmt.Errorf("failed to save bot config: %w", err)
	}
	return nil
}

func (s *Service) AddFact(factText string, tags []string) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		INSERT INTO facts (content, created_at)
		VALUES (?, CURRENT_TIMESTAMP)
	`, factText)
	if err != nil {
		return 0, fmt.Errorf("failed to insert fact: %w", err)
	}

	factID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}

	for _, tag := range tags {
		if tag == "" {
			continue
		}
		_, err = tx.Exec(`INSERT INTO fact_tags (fact_id, tag) VALUES (?, ?)`, factID, tag)
		if err != nil {
			return 0, fmt.Errorf("failed to insert tag %s: %w", tag, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return factID, nil
}

func (s *Service) RemoveFact(factID int) error {
	_, err := s.db.Exec(`DELETE FROM facts WHERE id = ?`, factID)
	if err != nil {
		return fmt.Errorf("failed to delete fact %d: %w", factID, err)
	}
	return nil
}

func (s *Service) SearchFacts(requiredTags []string, anyTags []string, limit int) ([]Fact, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive")
	}

	query := `
		SELECT DISTINCT f.id, f.content
		FROM facts f
		WHERE 1=1
	`
	args := []any{}

	if len(requiredTags) > 0 {
		query += ` AND f.id IN (
			SELECT fact_id FROM fact_tags
			WHERE tag IN (` + placeholders(len(requiredTags)) + `)
			GROUP BY fact_id
			HAVING COUNT(DISTINCT tag) = ?
		)`
		for _, tag := range requiredTags {
			args = append(args, tag)
		}
		args = append(args, len(requiredTags))
	}

	if len(anyTags) > 0 {
		query += ` AND f.id IN (
			SELECT DISTINCT fact_id FROM fact_tags
			WHERE tag IN (` + placeholders(len(anyTags)) + `)
		)`
		for _, tag := range anyTags {
			args = append(args, tag)
		}
	}

	query += ` ORDER BY RANDOM() LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search facts: %w", err)
	}
	defer rows.Close()

	var facts []Fact
	for rows.Next() {
		var f Fact
		if err := rows.Scan(&f.ID, &f.Content); err != nil {
			return nil, fmt.Errorf("failed to scan fact: %w", err)
		}

		tagRows, err := s.db.Query(`SELECT tag FROM fact_tags WHERE fact_id = ?`, f.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get tags for fact %d: %w", f.ID, err)
		}
		for tagRows.Next() {
			var tag string
			if err := tagRows.Scan(&tag); err != nil {
				tagRows.Close()
				return nil, fmt.Errorf("failed to scan tag: %w", err)
			}
			f.Tags = append(f.Tags, tag)
		}
		tagRows.Close()

		facts = append(facts, f)
	}

	return facts, nil
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	result := "?"
	for i := 1; i < n; i++ {
		result += ", ?"
	}
	return result
}
