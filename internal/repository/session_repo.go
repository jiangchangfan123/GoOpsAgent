package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/goopsagent/goopsagent/internal/models"
)

type SessionRepo struct {
	db *sql.DB
}

func NewSessionRepo(db *sql.DB) *SessionRepo {
	return &SessionRepo{db: db}
}

func (r *SessionRepo) Create(userID, title string) (*models.Session, error) {
	s := &models.Session{
		ID:        uuid.New().String(),
		UserID:    userID,
		Title:     title,
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := r.db.Exec(
		`INSERT INTO sessions (id, user_id, title, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		s.ID, s.UserID, s.Title, s.Status, s.CreatedAt, s.UpdatedAt,
	)
	return s, err
}

func (r *SessionRepo) GetByID(id string) (*models.Session, error) {
	s := &models.Session{}
	err := r.db.QueryRow(
		`SELECT id, user_id, title, status, created_at, updated_at FROM sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.UserID, &s.Title, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session %s not found", id)
		}
		return nil, err
	}
	return s, nil
}

func (r *SessionRepo) ListByUser(userID string) ([]models.Session, error) {
	rows, err := r.db.Query(
		`SELECT id, user_id, title, status, created_at, updated_at FROM sessions WHERE user_id = ? ORDER BY updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.Session
	for rows.Next() {
		var s models.Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.Title, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}
