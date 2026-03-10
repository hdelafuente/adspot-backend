CREATE TABLE IF NOT EXISTS adspots (
    id            TEXT PRIMARY KEY,
    title         TEXT NOT NULL,
    image_url     TEXT NOT NULL,
    placement     TEXT NOT NULL CHECK (placement IN ('home_screen', 'ride_summary', 'map_view')),
    status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at    TEXT NOT NULL,
    deactivated_at TEXT,
    ttl_minutes   INTEGER
);
