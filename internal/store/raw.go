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
// stopping on the first error. Used by renormalization.
func (s *Store) EachRawBatch(ctx context.Context, fn func(payload []byte) error) error {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM raw_batches ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return err
		}
		if err := fn(payload); err != nil {
			return err
		}
	}
	return rows.Err()
}
