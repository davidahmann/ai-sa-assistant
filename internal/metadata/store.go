package metadata

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// Store handles queries to the SQLite metadata database
type Store struct {
	db *sql.DB
}

// NewStore creates a new metadata store
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &Store{db: db}

	// Initialize database schema
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// initSchema creates the metadata table if it doesn't exist
func (s *Store) initSchema() error {
	query := `
		CREATE TABLE IF NOT EXISTS metadata (
			doc_id TEXT PRIMARY KEY,
			scenario TEXT,
			cloud TEXT,
			category TEXT,
			tags TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`

	_, err := s.db.Exec(query)
	return err
}

// MetadataEntry represents a metadata entry
type MetadataEntry struct {
	DocID    string `json:"doc_id"`
	Scenario string `json:"scenario"`
	Cloud    string `json:"cloud"`
	Category string `json:"category"`
	Tags     string `json:"tags"`
}

// AddMetadata adds a metadata entry to the database
func (s *Store) AddMetadata(entry MetadataEntry) error {
	query := `
		INSERT OR REPLACE INTO metadata (doc_id, scenario, cloud, category, tags)
		VALUES (?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query, entry.DocID, entry.Scenario, entry.Cloud, entry.Category, entry.Tags)
	if err != nil {
		return fmt.Errorf("failed to insert metadata: %w", err)
	}

	return nil
}

// FilterOptions represents filter criteria for metadata queries
type FilterOptions struct {
	Scenario string
	Cloud    string
	Category string
	Tags     []string
}

// QueryDocumentIDs returns document IDs matching the given filters
func (s *Store) QueryDocumentIDs(filters FilterOptions) ([]string, error) {
	var conditions []string
	var args []interface{}

	if filters.Scenario != "" {
		conditions = append(conditions, "scenario = ?")
		args = append(args, filters.Scenario)
	}

	if filters.Cloud != "" {
		conditions = append(conditions, "cloud = ?")
		args = append(args, filters.Cloud)
	}

	if filters.Category != "" {
		conditions = append(conditions, "category = ?")
		args = append(args, filters.Category)
	}

	if len(filters.Tags) > 0 {
		// Search for documents that contain any of the specified tags
		tagConditions := make([]string, len(filters.Tags))
		for i, tag := range filters.Tags {
			tagConditions[i] = "tags LIKE ?"
			args = append(args, "%"+tag+"%")
		}
		conditions = append(conditions, "("+strings.Join(tagConditions, " OR ")+")")
	}

	query := "SELECT doc_id FROM metadata"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query metadata: %w", err)
	}
	defer rows.Close()

	var docIDs []string
	for rows.Next() {
		var docID string
		if err := rows.Scan(&docID); err != nil {
			return nil, fmt.Errorf("failed to scan doc_id: %w", err)
		}
		docIDs = append(docIDs, docID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return docIDs, nil
}

// GetAllMetadata returns all metadata entries
func (s *Store) GetAllMetadata() ([]MetadataEntry, error) {
	query := "SELECT doc_id, scenario, cloud, category, tags FROM metadata"

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all metadata: %w", err)
	}
	defer rows.Close()

	var entries []MetadataEntry
	for rows.Next() {
		var entry MetadataEntry
		err := rows.Scan(&entry.DocID, &entry.Scenario, &entry.Cloud, &entry.Category, &entry.Tags)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metadata entry: %w", err)
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metadata rows: %w", err)
	}

	return entries, nil
}

// GetMetadataByDocID returns metadata for a specific document ID
func (s *Store) GetMetadataByDocID(docID string) (*MetadataEntry, error) {
	query := "SELECT doc_id, scenario, cloud, category, tags FROM metadata WHERE doc_id = ?"

	row := s.db.QueryRow(query, docID)

	var entry MetadataEntry
	err := row.Scan(&entry.DocID, &entry.Scenario, &entry.Cloud, &entry.Category, &entry.Tags)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Document not found
		}
		return nil, fmt.Errorf("failed to scan metadata: %w", err)
	}

	return &entry, nil
}
