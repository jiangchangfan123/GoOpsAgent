package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/goopsagent/goopsagent/internal/models"
	"github.com/google/uuid"
)

// ---- Session CRUD ----

// CreateSession 创建新会话
func (s *Store) CreateSession(userID, title string) (*models.Session, error) {
	session := &models.Session{
		ID:        uuid.New().String(),
		UserID:    userID,
		Title:     title,
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, user_id, title, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		session.ID, session.UserID, session.Title, session.Status, session.CreatedAt, session.UpdatedAt,
	)
	return session, err
}

// GetSession 按 ID 查询会话
func (s *Store) GetSession(id string) (*models.Session, error) {
	session := &models.Session{}
	err := s.db.QueryRow(
		`SELECT id, user_id, title, status, created_at, updated_at FROM sessions WHERE id = ?`, id,
	).Scan(&session.ID, &session.UserID, &session.Title, &session.Status, &session.CreatedAt, &session.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session %s not found", id)
		}
		return nil, err
	}
	return session, nil
}

// ListSessionsByUser 查询用户的所有会话
func (s *Store) ListSessionsByUser(userID string) ([]models.Session, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, title, status, created_at, updated_at FROM sessions WHERE user_id = ? ORDER BY updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.Session
	for rows.Next() {
		var sess models.Session
		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.Title, &sess.Status, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}