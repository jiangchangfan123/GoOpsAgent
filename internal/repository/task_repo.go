package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
	"uuid"

	"github.com/goopsagent/goopsagent/internal/models"
)

type TaskRepo struct {
	db *sql.DB
}

func NewTaskRepo(db *sql.DB) *TaskRepo {
	return &TaskRepo{db: db}
}

func (r *TaskRepo) Create(sessionID, message string) (*models.Task, error) {
	task := &models.Task{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Message:   message,
		Status:    models.TaskStatusCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := r.db.Exec(
		`INSERT INTO tasks (id, session_id, message, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		task.ID, task.SessionID, task.Message, task.Status, task.CreatedAt, task.UpdatedAt,
	)
	return task, err
}

// UpdateStatus 更新任务状态
func (r *TaskRepo) UpdateStatus(id string, status models.TaskStatus) error {
	_, err := r.db.Exec(
		`UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now(), id,
	)
	return err
}

// Complete 任务完成，写入答案和工具调用记录
func (r *TaskRepo) Complete(id string, answer string, toolRuns []models.ToolRun) error {
	trJSON, _ := json.Marshal(toolRuns)
	_, err := r.db.Exec(
		`UPDATE tasks SET status = ?, answer = ?, tool_runs = ?, updated_at = ? WHERE id = ?`,
		models.TaskStatusSucceeded, answer, trJSON, time.Now(), id,
	)
	return err
}

// Fail 标记任务失败
func (r *TaskRepo) Fail(id, errMsg string) error {
	_, err := r.db.Exec(
		`UPDATE tasks SET status = ?, error = ?, updated_at = ? WHERE id = ?`,
		models.TaskStatusFailed, errMsg, time.Now(), id,
	)
	return err
}

// GetByID 按 ID 查询任务
func (r *TaskRepo) GetByID(id string) (*models.Task, error) {
	task := &models.Task{}
	var toolRunsJSON sql.NullString
	var answer, errMsg sql.NullString

	err := r.db.QueryRow(
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
