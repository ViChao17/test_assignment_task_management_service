// Package model содержит модели, общие для слоёв сервисов.
package model

import (
	"encoding/json"
	"time"
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

const (
	StatusTodo       = "todo"
	StatusInProgress = "in_progress"
	StatusDone       = "done"
)

var TaskStatuses = []string{StatusTodo, StatusInProgress, StatusDone}

func IsValidRole(role string) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleMember:
		return true
	default:
		return false
	}
}

func IsValidStatus(status string) bool {
	switch status {
	case StatusTodo, StatusInProgress, StatusDone:
		return true
	default:
		return false
	}
}

type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
}

type Team struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	CreatedBy   int64     `json:"created_by"`
	Role        string    `json:"role"`
	MemberCount int       `json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type TeamMember struct {
	TeamID   int64     `json:"team_id"`
	UserID   int64     `json:"user_id"`
	Role     string    `json:"role"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	JoinedAt time.Time `json:"joined_at"`
}

type Task struct {
	ID           int64      `json:"id"`
	TeamID       int64      `json:"team_id"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Status       string     `json:"status"`
	CreatedBy    int64      `json:"created_by"`
	CreatorName  string     `json:"creator_name"`
	AssigneeID   *int64     `json:"assignee_id"`
	AssigneeName *string    `json:"assignee_name"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ClosedAt     *time.Time `json:"closed_at"`
	Version      int64      `json:"version"`
}

type TaskFilter struct {
	TeamID     int64
	Status     string
	AssigneeID *int64
	Limit      int
	Offset     int
}

type TaskList struct {
	Items  []Task `json:"items"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type TaskHistory struct {
	ID            int64           `json:"id"`
	TaskID        int64           `json:"task_id"`
	ChangedBy     int64           `json:"changed_by"`
	ChangedByName string          `json:"changed_by_name"`
	Changes       json.RawMessage `json:"changes"`
	CreatedAt     time.Time       `json:"created_at"`
}

type Comment struct {
	ID        int64     `json:"id"`
	TaskID    int64     `json:"task_id"`
	UserID    int64     `json:"user_id"`
	UserName  string    `json:"user_name"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type AssigneeStat struct {
	UserID         int64  `json:"user_id"`
	Name           string `json:"name"`
	ClosedTasks30d int64  `json:"closed_tasks_30d"`
}

type TaskCommentsStat struct {
	TaskID       int64  `json:"task_id"`
	Title        string `json:"title"`
	CommentCount int64  `json:"comment_count"`
}

type TeamStats struct {
	TeamID                int64              `json:"team_id"`
	GeneratedAt           time.Time          `json:"generated_at"`
	TotalTasks            int64              `json:"total_tasks"`
	TasksByStatus         map[string]int64   `json:"tasks_by_status"`
	TopAssignees30d       []AssigneeStat     `json:"top_assignees_30d"`
	AverageCloseTimeHours *float64           `json:"average_close_time_hours"`
	CommentsByTask        []TaskCommentsStat `json:"comments_by_task"`
}
