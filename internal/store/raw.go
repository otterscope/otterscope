package store

import (
	"context"
	"fmt"
	"time"

	"github.com/otterscope/otterscope/internal/model"
)

// IngestBatch persists a raw ingested batch and its normalized steps in one
// transaction. raw is the gzip-compressed protobuf of the original request.
func (s *Store) IngestBatch(ctx context.Context, raw []byte, steps []model.Step) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO raw_batches (received_ns, payload) VALUES (?, ?)`,
		time.Now().UnixNano(), raw); err != nil {
		return fmt.Errorf("insert raw batch: %w", err)
	}
	if err := upsertStepsTx(ctx, tx, steps); err != nil {
		return err
	}
	return tx.Commit()
}

// EachRawBatch streams every stored raw payload to fn in insertion order,
// stopping on the first error.
//
// Payloads are fetched in pages and each page's rows cursor is fully drained
// and closed BEFORE fn runs: the store has a single SQLite connection, so a
// cursor held open across a write inside fn would self-deadlock.
func (s *Store) EachRawBatch(ctx context.Context, fn func(payload []byte) error) error {
	const pageSize = 100
	lastID := int64(0)
	for {
		payloads, maxID, err := s.rawBatchPage(ctx, lastID, pageSize)
		if err != nil {
			return err
		}
		if len(payloads) == 0 {
			return nil
		}
		for _, p := range payloads {
			if err := fn(p); err != nil {
				return err
			}
		}
		lastID = maxID
	}
}

func (s *Store) rawBatchPage(ctx context.Context, afterID int64, limit int) ([][]byte, int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, payload FROM raw_batches WHERE id > ? ORDER BY id LIMIT ?`, afterID, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var payloads [][]byte
	var maxID int64
	for rows.Next() {
		var id int64
		var payload []byte
		if err := rows.Scan(&id, &payload); err != nil {
			return nil, 0, err
		}
		payloads = append(payloads, payload)
		maxID = id
	}
	return payloads, maxID, rows.Err()
}
