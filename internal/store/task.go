package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/goopsagent/goopsagent/internal/models"
	"github.com/google/uuid"
)

// ---- Task CRUD ----

// CreateTask 创建新任务
func (s *Store) CreateTask(sessionID, message string) (*models.Task, error) {
	task := &models.Task{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Message:   message,
		Status:    models.TaskStatusCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := s.db.Exec(
		`INSERT INTO tasks (id, session_id, message, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		task.ID, task.SessionID, task.Message, task.Status, task.CreatedAt, task.UpdatedAt,
	)
	return task, err
}

// UpdateTaskStatus 更新任务状态
func (s *Store) UpdateTaskStatus(id string, status models.TaskStatus) error {
	_, err := s.db.Exec(
		`UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now(), id,
	)
	return err
}

// CompleteTask 任务完成，写入答案和工具调用记录
func (s *Store) CompleteTask(id string, answer string, toolRuns []models.ToolRun) error {
	trJSON, _ := json.Marshal(toolRuns)
	_, err := s.db.Exec(
		`UPDATE tasks SET status = ?, answer = ?, tool_runs = ?, updated_at = ? WHERE id = ?`,
		models.TaskStatusSucceeded, answer, trJSON, time.Now(), id,
	)
	return err
}

// FailTask 标记任务失败
func (s *Store) FailTask(id, errMsg string) error {
	_, err := s.db.Exec(
		`UPDATE tasks SET status = ?, error = ?, updated_at = ? WHERE id = ?`,
		models.TaskStatusFailed, errMsg, time.Now(), id,
	)
	return err
}

// GetTask 按 ID 查询任务
func (s *Store) GetTask(id string) (*models.Task, error) {
	task := &models.Task{}
	var toolRunsJSON sql.NullString
	var answer, errMsg sql.NullString

	err := s.db.QueryRow(
		`SELECT id, session_id, message, status, answer, tool_runs, error, created_at, updated_at FROM tasks WHERE id = ?`,
		id,
	).Scan(&task.ID, &task.SessionID, &task.Message, &task.Status,
		&answer, &toolRunsJSON, &errMsg, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("task %s not found", id)
		}
		return nil, err
	}

	task.Answer = answer.String
	task.Error = errMsg.String
	if toolRunsJSON.Valid {
		json.Unmarshal([]byte(toolRunsJSON.String), &task.ToolRuns)
	}
	return task, nil
}