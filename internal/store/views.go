package store

import (
	"context"
	"encoding/json"
	"time"
)

// SavedView is a named filter/search combination.
type SavedView struct {
	ID     int64           `json:"id"`
	Name   string          `json:"name"`
	Params json.RawMessage `json:"params"`
}

// CreateSavedView stores a view. params is opaque JSON (the UI's filter state).
func (s *Store) CreateSavedView(ctx context.Context, name string, params json.RawMessage) (SavedView, error) {
	if len(params) == 0 {
		params = json.RawMessage("{}")
	}
	res, err := s.writer.ExecContext(ctx,
		`INSERT INTO saved_views (name, params, created_ns) VALUES (?,?,?)`,
		name, string(params), time.Now().UnixNano())
	if err != nil {
		return SavedView{}, err
	}
	id, _ := res.LastInsertId()
	return SavedView{ID: id, Name: name, Params: params}, nil
}

// ListSavedViews returns all views, oldest first.
func (s *Store) ListSavedViews(ctx context.Context) ([]SavedView, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT id, name, params FROM saved_views ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SavedView
	for rows.Next() {
		var v SavedView
		var params string
		if err := rows.Scan(&v.ID, &v.Name, &params); err != nil {
			return nil, err
		}
		v.Params = json.RawMessage(params)
		out = append(out, v)
	}
	return out, rows.Err()
}

// DeleteSavedView removes a view.
func (s *Store) DeleteSavedView(ctx context.Context, id int64) error {
	_, err := s.writer.ExecContext(ctx, `DELETE FROM saved_views WHERE id = ?`, id)
	return err
}
