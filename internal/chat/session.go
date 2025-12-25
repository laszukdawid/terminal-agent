package chat

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);
`

// Message represents a chat message
type Message struct {
	Role    string
	Content string
}

// SessionStore manages chat sessions and messages
type SessionStore struct {
	db              *sql.DB
	currentSession  int64
	sessionFilePath string
}

// NewSessionStore creates a new session store with the given database path
func NewSessionStore(dbPath string) (*SessionStore, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create schema
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	sessionFilePath := filepath.Join(dir, "current_session")

	store := &SessionStore{
		db:              db,
		sessionFilePath: sessionFilePath,
	}

	// Load current session ID
	if err := store.loadCurrentSession(); err != nil {
		// No current session, that's fine
		store.currentSession = 0
	}

	return store, nil
}

// Close closes the database connection
func (s *SessionStore) Close() error {
	return s.db.Close()
}

// loadCurrentSession reads the current session ID from file
func (s *SessionStore) loadCurrentSession() error {
	data, err := os.ReadFile(s.sessionFilePath)
	if err != nil {
		return err
	}

	var sessionID int64
	if _, err := fmt.Sscanf(string(data), "%d", &sessionID); err != nil {
		return err
	}

	// Verify session exists
	var exists bool
	err = s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM sessions WHERE id = ?)", sessionID).Scan(&exists)
	if err != nil || !exists {
		return fmt.Errorf("session does not exist")
	}

	s.currentSession = sessionID
	return nil
}

// saveCurrentSession writes the current session ID to file
func (s *SessionStore) saveCurrentSession() error {
	return os.WriteFile(s.sessionFilePath, []byte(fmt.Sprintf("%d", s.currentSession)), 0644)
}

// NewSession creates a new session and sets it as current
func (s *SessionStore) NewSession() (int64, error) {
	result, err := s.db.Exec("INSERT INTO sessions DEFAULT VALUES")
	if err != nil {
		return 0, fmt.Errorf("failed to create session: %w", err)
	}

	sessionID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get session ID: %w", err)
	}

	s.currentSession = sessionID
	if err := s.saveCurrentSession(); err != nil {
		return 0, fmt.Errorf("failed to save current session: %w", err)
	}

	return sessionID, nil
}

// GetOrCreateSession returns the current session or creates a new one
func (s *SessionStore) GetOrCreateSession() (int64, error) {
	if s.currentSession > 0 {
		return s.currentSession, nil
	}
	return s.NewSession()
}

// AddMessage adds a message to the current session
func (s *SessionStore) AddMessage(role, content string) error {
	if s.currentSession == 0 {
		return fmt.Errorf("no active session")
	}

	_, err := s.db.Exec(
		"INSERT INTO messages (session_id, role, content) VALUES (?, ?, ?)",
		s.currentSession, role, content,
	)
	if err != nil {
		return fmt.Errorf("failed to add message: %w", err)
	}

	return nil
}

// GetMessages returns all messages for the current session
func (s *SessionStore) GetMessages() ([]Message, error) {
	if s.currentSession == 0 {
		return []Message{}, nil
	}

	rows, err := s.db.Query(
		"SELECT role, content FROM messages WHERE session_id = ? ORDER BY id ASC",
		s.currentSession,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

// CurrentSessionID returns the current session ID
func (s *SessionStore) CurrentSessionID() int64 {
	return s.currentSession
}
