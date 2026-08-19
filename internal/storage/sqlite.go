package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
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

func (s *Store) Close() error {
	return s.db.Close()
}