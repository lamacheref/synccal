package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type CalendarState struct {
	CTag      string
	SyncToken string
	ETag      string
}

func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS event_mapping (
		source_uid TEXT NOT NULL,
		dest_name TEXT NOT NULL,
		dest_uid TEXT NOT NULL,
		dest_href TEXT NOT NULL,
		dest_etag TEXT,
		content_hash TEXT NOT NULL,
		synced_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		deleted BOOLEAN DEFAULT FALSE,
		PRIMARY KEY (source_uid, dest_name)
	);
	CREATE INDEX IF NOT EXISTS idx_event_mapping_dest ON event_mapping(dest_name, dest_uid);

	CREATE TABLE IF NOT EXISTS source_state (
		source_url TEXT PRIMARY KEY,
		ctag TEXT,
		sync_token TEXT,
		etag TEXT,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) GetMapping(sourceUID, destName string) (string, string, string, bool, error) {
	var destUID, destHref, destETag, contentHash string
	var deleted bool
	err := s.db.QueryRow(
		`SELECT dest_uid, dest_href, dest_etag, content_hash, deleted FROM event_mapping WHERE source_uid = ? AND dest_name = ?`,
		sourceUID, destName,
	).Scan(&destUID, &destHref, &destETag, &contentHash, &deleted)
	if err == sql.ErrNoRows {
		return "", "", "", false, nil
	}
	if err != nil {
		return "", "", "", false, err
	}
	return destUID, destHref, destETag, deleted, nil
}

func (s *Store) SetMapping(sourceUID, destName, destUID, destHref, destETag, contentHash string, deleted bool) error {
	_, err := s.db.Exec(`
		INSERT INTO event_mapping (source_uid, dest_name, dest_uid, dest_href, dest_etag, content_hash, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_uid, dest_name) DO UPDATE SET
			dest_uid = excluded.dest_uid,
			dest_href = excluded.dest_href,
			dest_etag = excluded.dest_etag,
			content_hash = excluded.content_hash,
			deleted = excluded.deleted,
			synced_at = CURRENT_TIMESTAMP
	`, sourceUID, destName, destUID, destHref, destETag, contentHash, deleted)
	return err
}

func (s *Store) ListMappings(destName string) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT source_uid, content_hash FROM event_mapping WHERE dest_name = ? AND deleted = FALSE`, destName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var sourceUID, hash string
		if err := rows.Scan(&sourceUID, &hash); err != nil {
			return nil, err
		}
		m[sourceUID] = hash
	}
	return m, nil
}

type EventRecord struct {
	SourceUID   string `json:"source_uid"`
	DestName    string `json:"dest_name"`
	DestUID     string `json:"dest_uid"`
	ContentHash string `json:"content_hash"`
	SyncedAt    string `json:"synced_at"`
	Deleted     bool   `json:"deleted"`
}

// ListEvents returns mapped events, optionally filtered by destination,
// newest first, with basic pagination.
func (s *Store) ListEvents(destName string, limit, offset int) ([]EventRecord, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	query := `SELECT source_uid, dest_name, dest_uid, content_hash, synced_at, deleted
		FROM event_mapping`
	args := make([]interface{}, 0, 3)
	if destName != "" {
		query += ` WHERE dest_name = ?`
		args = append(args, destName)
	}
	query += ` ORDER BY synced_at DESC, source_uid LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EventRecord
	for rows.Next() {
		var rec EventRecord
		if err := rows.Scan(&rec.SourceUID, &rec.DestName, &rec.DestUID, &rec.ContentHash, &rec.SyncedAt, &rec.Deleted); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) GetSourceState(sourceURL string) (*CalendarState, error) {
	var state CalendarState
	err := s.db.QueryRow(
		`SELECT ctag, sync_token, etag FROM source_state WHERE source_url = ?`,
		sourceURL,
	).Scan(&state.CTag, &state.SyncToken, &state.ETag)
	if err == sql.ErrNoRows {
		return &CalendarState{}, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *Store) SetSourceState(sourceURL string, state *CalendarState) error {
	_, err := s.db.Exec(`
		INSERT INTO source_state (source_url, ctag, sync_token, etag)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(source_url) DO UPDATE SET
			ctag = excluded.ctag,
			sync_token = excluded.sync_token,
			etag = excluded.etag,
			updated_at = CURRENT_TIMESTAMP
	`, sourceURL, state.CTag, state.SyncToken, state.ETag)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping() error {
	return s.db.Ping()
}
