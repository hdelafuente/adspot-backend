package adspot

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"
)

// Repository handles persistence for AdSpots.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new Repository backed by db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Create inserts a new active AdSpot and returns it.
func (r *Repository) Create(ctx context.Context, req CreateRequest) (*AdSpot, error) {
	spot := &AdSpot{
		ID:         newID(),
		Title:      req.Title,
		ImageURL:   req.ImageURL,
		Placement:  req.Placement,
		Status:     StatusActive,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		TTLMinutes: req.TTLMinutes,
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO adspots (id, title, image_url, placement, status, created_at, ttl_minutes)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		spot.ID, spot.Title, spot.ImageURL, spot.Placement,
		spot.Status, spot.CreatedAt, spot.TTLMinutes,
	)
	if err != nil {
		return nil, fmt.Errorf("insert adspot: %w", err)
	}
	return spot, nil
}

// GetByID returns the AdSpot with the given id, or nil if not found.
func (r *Repository) GetByID(ctx context.Context, id string) (*AdSpot, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, title, image_url, placement, status, created_at, deactivated_at, ttl_minutes
		 FROM adspots WHERE id = ?`, id)
	return scanRow(row)
}

// Deactivate marks the AdSpot as inactive and sets deactivated_at to now.
func (r *Repository) Deactivate(ctx context.Context, id string) (*AdSpot, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := r.db.ExecContext(ctx,
		`UPDATE adspots SET status = 'inactive', deactivated_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return nil, fmt.Errorf("deactivate adspot: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, nil // not found
	}
	return r.GetByID(ctx, id)
}

// ListEligible returns active, non-expired AdSpots grouped by placement.
// If placement is non-empty only that placement is returned.
func (r *Repository) ListEligible(ctx context.Context, placement string) (map[string][]*AdSpot, error) {
	q := `
		SELECT id, title, image_url, placement, status, created_at, deactivated_at, ttl_minutes
		FROM adspots
		WHERE status = 'active'
		  AND (ttl_minutes IS NULL
		       OR datetime(created_at, '+' || ttl_minutes || ' minutes') > datetime('now'))`

	args := []any{}
	if placement != "" {
		q += " AND placement = ?"
		args = append(args, placement)
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list eligible: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]*AdSpot)
	for rows.Next() {
		spot, err := scanRows(rows)
		if err != nil {
			return nil, err
		}
		result[spot.Placement] = append(result[spot.Placement], spot)
	}
	return result, rows.Err()
}

// ── helpers ──────────────────────────────────────────────────────────────────

func scanRow(row *sql.Row) (*AdSpot, error) {
	var s AdSpot
	var deactivatedAt sql.NullString
	var ttl sql.NullInt64
	err := row.Scan(&s.ID, &s.Title, &s.ImageURL, &s.Placement,
		&s.Status, &s.CreatedAt, &deactivatedAt, &ttl)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan adspot: %w", err)
	}
	applyNulls(&s, deactivatedAt, ttl)
	return &s, nil
}

func scanRows(rows *sql.Rows) (*AdSpot, error) {
	var s AdSpot
	var deactivatedAt sql.NullString
	var ttl sql.NullInt64
	if err := rows.Scan(&s.ID, &s.Title, &s.ImageURL, &s.Placement,
		&s.Status, &s.CreatedAt, &deactivatedAt, &ttl); err != nil {
		return nil, fmt.Errorf("scan adspot row: %w", err)
	}
	applyNulls(&s, deactivatedAt, ttl)
	return &s, nil
}

func applyNulls(s *AdSpot, deactivatedAt sql.NullString, ttl sql.NullInt64) {
	if deactivatedAt.Valid {
		s.DeactivatedAt = &deactivatedAt.String
	}
	if ttl.Valid {
		v := int(ttl.Int64)
		s.TTLMinutes = &v
	}
}

// newID generates a random UUID v4-formatted string.
func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
