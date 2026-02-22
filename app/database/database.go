package database

import (
	"context"
	"database/sql"
	"durkalive/app/config"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	_ "embed"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
	"github.com/samber/do"
)

//go:embed schema.sql
var schemaStr string

var _ do.Shutdownable = (*Service)(nil)

type Service struct {
	db *pgxpool.Pool
}

func New(di *do.Injector) (*Service, error) {
	appCtx := do.MustInvoke[context.Context](di)
	cfg := do.MustInvoke[*config.Config](di)

	dbConnStr := "postgres://" + cfg.DB.User + ":" + cfg.DB.Pass + "@" + cfg.DB.Host + "/" + cfg.DB.Database + "?sslmode=disable&pool_max_conns=30&pool_min_conns=5&pool_max_conn_lifetime=1h&pool_max_conn_idle_time=30m&pool_health_check_period=1m&connect_timeout=10"

	dbConf, err := pgxpool.ParseConfig(dbConnStr)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.ParseConfig: %w", err)
	}

	dbConf.ConnConfig.RuntimeParams = map[string]string{
		"statement_timeout":                   "30000",
		"idle_in_transaction_session_timeout": "60000",
	}

	db, err := pgxpool.NewWithConfig(appCtx, dbConf)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.NewWithConfig: %w", err)
	}

	if err = db.Ping(appCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if _, err := db.Exec(appCtx, schemaStr); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return &Service{db: db}, nil
}

func (s *Service) Shutdown() error {
	s.db.Close()

	return nil
}

func (s *Service) GetBotConfig(ctx context.Context) (*BotConfigRow, error) {
	var row BotConfigRow
	err := s.db.QueryRow(ctx, `SELECT id, data FROM bot_config WHERE id = 1`).Scan(
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

func (s *Service) SaveBotConfig(ctx context.Context, data any) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal bot config: %w", err)
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO bot_config (id, data) VALUES (1, $1)
		ON CONFLICT (id) DO UPDATE SET data = $1
	`, string(jsonBytes))
	if err != nil {
		return fmt.Errorf("failed to save bot config: %w", err)
	}
	return nil
}

func (s *Service) AddFact(ctx context.Context, factText string, tags []string, usernames []string, embedding pgvector.Vector) (int64, error) {
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal tags: %w", err)
	}

	usernamesJSON, err := json.Marshal(usernames)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal usernames: %w", err)
	}

	var id int64
	err = s.db.QueryRow(ctx, `
		INSERT INTO facts (content, tags, usernames, embedding, created_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		RETURNING id
	`, factText, string(tagsJSON), string(usernamesJSON), embedding).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to insert fact: %w", err)
	}

	return id, nil
}

func (s *Service) RemoveFact(ctx context.Context, factID int) error {
	_, err := s.db.Exec(ctx, `DELETE FROM facts WHERE id = $1`, factID)
	if err != nil {
		return fmt.Errorf("failed to delete fact %d: %w", factID, err)
	}
	return nil
}

func (s *Service) SearchFacts(ctx context.Context, requiredTags []string, anyTags []string, usernames []string, limit int) ([]Fact, error) {
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

	rows, err := s.db.Query(ctx, query, args...)
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

func (s *Service) FindSimilarFacts(ctx context.Context, embedding pgvector.Vector, threshold float32, limit int) ([]SimilarFact, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, content, 1 - (embedding <=> $1) AS similarity
		FROM facts
		WHERE embedding IS NOT NULL AND 1 - (embedding <=> $1) > $2
		ORDER BY similarity DESC
		LIMIT $3
	`, embedding, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find similar facts: %w", err)
	}
	defer rows.Close()

	var facts []SimilarFact
	for rows.Next() {
		var f SimilarFact
		if err := rows.Scan(&f.ID, &f.Content, &f.Similarity); err != nil {
			return nil, fmt.Errorf("failed to scan similar fact: %w", err)
		}
		facts = append(facts, f)
	}
	return facts, nil
}
