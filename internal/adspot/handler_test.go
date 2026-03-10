package adspot_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	chi "github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"

	"github.com/adspot-backend/adspot-backend/internal/adspot"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS adspots (
			id            TEXT PRIMARY KEY,
			title         TEXT NOT NULL,
			image_url     TEXT NOT NULL,
			placement     TEXT NOT NULL,
			status        TEXT NOT NULL DEFAULT 'active',
			created_at    TEXT NOT NULL,
			deactivated_at TEXT,
			ttl_minutes   INTEGER
		)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insert(t *testing.T, db *sql.DB, id, title, placement, status string, createdAt time.Time, ttl *int) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO adspots (id, title, image_url, placement, status, created_at, ttl_minutes)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, title, "http://img.test/"+id+".png", placement, status,
		createdAt.UTC().Format(time.RFC3339), ttlVal(ttl),
	)
	if err != nil {
		t.Fatalf("insert fixture %s: %v", id, err)
	}
}

func ttlVal(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func newRouter(db *sql.DB) http.Handler {
	r := chi.NewRouter()
	r.Mount("/adspots", adspot.NewHandler(adspot.NewRepository(db)).Routes())
	return r
}

// TestListEligible_OnlyReturnsActiveNonExpired verifies the GET /adspots endpoint:
//   - excludes spots whose status is 'inactive'
//   - excludes spots whose TTL has already elapsed
//   - includes spots that are active and within their TTL
//   - groups results by placement
func TestListEligible_OnlyReturnsActiveNonExpired(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()

	ttl5 := 5
	ttl60 := 60

	// ── fixtures ──────────────────────────────────────────────────────────────
	// Should appear: active, no TTL
	insert(t, db, "active-no-ttl", "Active No TTL", "home_screen", "active", now, nil)
	// Should appear: active, TTL not yet expired (created now, expires in 60 min)
	insert(t, db, "active-ttl-ok", "Active TTL OK", "home_screen", "active", now, &ttl60)
	// Should NOT appear: inactive (manually deactivated)
	insert(t, db, "inactive-manual", "Inactive Manual", "home_screen", "inactive", now, nil)
	// Should NOT appear: active but TTL already expired (created 10 min ago, TTL 5 min)
	insert(t, db, "expired-ttl", "Expired TTL", "ride_summary", "active", now.Add(-10*time.Minute), &ttl5)
	// Should appear: different placement, active, no TTL
	insert(t, db, "map-view-active", "Map View Active", "map_view", "active", now, nil)

	// ── request ───────────────────────────────────────────────────────────────
	req := httptest.NewRequest(http.MethodGet, "/adspots?status=active", nil)
	w := httptest.NewRecorder()
	newRouter(db).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d — body: %s", w.Code, w.Body.String())
	}

	// ── decode ────────────────────────────────────────────────────────────────
	var result map[string][]*adspot.AdSpot
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// ── assertions ────────────────────────────────────────────────────────────
	t.Run("home_screen has exactly 2 eligible ads", func(t *testing.T) {
		spots := result["home_screen"]
		if len(spots) != 2 {
			t.Errorf("want 2 home_screen spots, got %d", len(spots))
		}
		ids := map[string]bool{}
		for _, s := range spots {
			ids[s.ID] = true
		}
		if !ids["active-no-ttl"] {
			t.Error("expected 'active-no-ttl' to be present")
		}
		if !ids["active-ttl-ok"] {
			t.Error("expected 'active-ttl-ok' to be present")
		}
		if ids["inactive-manual"] {
			t.Error("'inactive-manual' must not appear")
		}
	})

	t.Run("ride_summary has no eligible ads (TTL expired)", func(t *testing.T) {
		spots := result["ride_summary"]
		if len(spots) != 0 {
			t.Errorf("want 0 ride_summary spots, got %d", len(spots))
		}
	})

	t.Run("map_view has exactly 1 eligible ad", func(t *testing.T) {
		spots := result["map_view"]
		if len(spots) != 1 {
			t.Errorf("want 1 map_view spot, got %d", len(spots))
		}
		if len(spots) > 0 && spots[0].ID != "map-view-active" {
			t.Errorf("want id 'map-view-active', got %s", spots[0].ID)
		}
	})

	t.Run("all returned spots have status active", func(t *testing.T) {
		for placement, spots := range result {
			for _, s := range spots {
				if s.Status != adspot.StatusActive {
					t.Errorf("placement %s: spot %s has status %s, want active",
						placement, s.ID, s.Status)
				}
			}
		}
	})
}

// TestListEligible_FilterByPlacement verifies that the placement query param narrows results.
func TestListEligible_FilterByPlacement(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()

	insert(t, db, "home-1", "Home 1", "home_screen", "active", now, nil)
	insert(t, db, "ride-1", "Ride 1", "ride_summary", "active", now, nil)

	req := httptest.NewRequest(http.MethodGet, "/adspots?placement=home_screen", nil)
	w := httptest.NewRecorder()
	newRouter(db).ServeHTTP(w, req)

	var result map[string][]*adspot.AdSpot
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if _, ok := result["ride_summary"]; ok {
		t.Error("ride_summary should not be returned when filtering by home_screen")
	}
	if len(result["home_screen"]) != 1 {
		t.Errorf("want 1 home_screen spot, got %d", len(result["home_screen"]))
	}
}
