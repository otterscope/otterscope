package store

import (
	"context"
	"fmt"
	"time"

	"github.com/otterscope/otterscope/internal/model"
)

// IngestBatch persists a raw ingested batch and its normalized steps in one
// transaction. raw is the gzip-compressed protobuf of the original request.
func (s *Store) IngestBatch(ctx context.Context, project string, raw []byte, steps []model.Step) error {
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO raw_batches (received_ns, payload, project) VALUES (?, ?, ?)`,
		time.Now().UnixNano(), raw, project); err != nil {
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
func (s *Store) EachRawBatch(ctx context.Context, fn func(project string, payload []byte) error) error {
	const pageSize = 100
	lastID := int64(0)
	for {
		batches, maxID, err := s.rawBatchPage(ctx, lastID, pageSize)
		if err != nil {
			return err
		}
		if len(batches) == 0 {
			return nil
		}
		for _, b := range batches {
			if err := fn(b.project, b.payload); err != nil {
				return err
			}
		}
		lastID = maxID
	}
}

type rawBatch struct {
	project string
	payload []byte
}

func (s *Store) rawBatchPage(ctx context.Context, afterID int64, limit int) ([]rawBatch, int64, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT id, project, payload FROM raw_batches WHERE id > ? ORDER BY id LIMIT ?`, afterID, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var batches []rawBatch
	var maxID int64
	for rows.Next() {
		var id int64
		var b rawBatch
		if err := rows.Scan(&id, &b.project, &b.payload); err != nil {
			return nil, 0, err
		}
		batches = append(batches, b)
		maxID = id
	}
	return batches, maxID, rows.Err()
}
