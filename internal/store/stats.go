package store

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"time"

	"taskmanager/internal/model"
)

// TeamStats выполняет один запрос со всеми секциями отчёта:
// без N+1 и с фильтром по team_id в каждом запросе.
func (s *Store) TeamStats(ctx context.Context, teamID int64) (*model.TeamStats, error) {
	rows, err := s.q(ctx).QueryContext(ctx, teamStatsQuery, teamID, teamID, teamID, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := &model.TeamStats{
		TeamID:          teamID,
		GeneratedAt:     time.Now().UTC(),
		TasksByStatus:   map[string]int64{model.StatusTodo: 0, model.StatusInProgress: 0, model.StatusDone: 0},
		TopAssignees30d: make([]model.AssigneeStat, 0),
		CommentsByTask:  make([]model.TaskCommentsStat, 0),
	}

	for rows.Next() {
		var (
			section   string
			label     sql.NullString
			userName  sql.NullString
			taskTitle sql.NullString
			userID    sql.NullInt64
			taskID    sql.NullInt64
			metric    sql.NullInt64
			avgHours  sql.NullFloat64
		)
		if err := rows.Scan(&section, &label, &userID, &userName, &taskID, &taskTitle, &metric, &avgHours); err != nil {
			return nil, err
		}

		switch section {
		case "status":
			if label.Valid && metric.Valid {
				stats.TasksByStatus[label.String] = metric.Int64
				stats.TotalTasks += metric.Int64
			}
		case "top_assignee":
			if userID.Valid && userName.Valid && metric.Valid {
				stats.TopAssignees30d = append(stats.TopAssignees30d, model.AssigneeStat{
					UserID:         userID.Int64,
					Name:           userName.String,
					ClosedTasks30d: metric.Int64,
				})
			}
		case "avg_close_time":
			if avgHours.Valid {
				value := math.Round(avgHours.Float64*100) / 100
				stats.AverageCloseTimeHours = &value
			}
		case "task_comments":
			if taskID.Valid && taskTitle.Valid && metric.Valid {
				stats.CommentsByTask = append(stats.CommentsByTask, model.TaskCommentsStat{
					TaskID:       taskID.Int64,
					Title:        taskTitle.String,
					CommentCount: metric.Int64,
				})
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(stats.TopAssignees30d, func(i, j int) bool {
		if stats.TopAssignees30d[i].ClosedTasks30d == stats.TopAssignees30d[j].ClosedTasks30d {
			return stats.TopAssignees30d[i].UserID < stats.TopAssignees30d[j].UserID
		}
		return stats.TopAssignees30d[i].ClosedTasks30d > stats.TopAssignees30d[j].ClosedTasks30d
	})
	sort.SliceStable(stats.CommentsByTask, func(i, j int) bool {
		if stats.CommentsByTask[i].CommentCount == stats.CommentsByTask[j].CommentCount {
			return stats.CommentsByTask[i].TaskID < stats.CommentsByTask[j].TaskID
		}
		return stats.CommentsByTask[i].CommentCount > stats.CommentsByTask[j].CommentCount
	})
	return stats, nil
}

const teamStatsQuery = `
WITH
status_counts AS (
    SELECT status, COUNT(*) AS task_count
    FROM tasks
    WHERE team_id = ?
    GROUP BY status
),
recent_closed AS (
    SELECT t.assignee_id, u.name AS assignee_name, COUNT(*) AS closed_count
    FROM tasks t
    INNER JOIN users u ON u.id = t.assignee_id
    WHERE t.team_id = ?
      AND t.status = 'done'
      AND t.assignee_id IS NOT NULL
      AND t.closed_at IS NOT NULL
      AND t.closed_at >= (UTC_TIMESTAMP() - INTERVAL 30 DAY)
    GROUP BY t.assignee_id, u.name
    ORDER BY closed_count DESC, t.assignee_id ASC
    LIMIT 3
),
close_time AS (
    SELECT AVG(TIMESTAMPDIFF(SECOND, t.created_at, t.closed_at)) / 3600.0 AS avg_hours
    FROM tasks t
    WHERE t.team_id = ?
      AND t.status = 'done'
      AND t.closed_at IS NOT NULL
),
comment_counts AS (
    SELECT t.id AS task_id, t.title, COUNT(c.id) AS comment_count
    FROM tasks t
    LEFT JOIN task_comments c ON c.task_id = t.id
    WHERE t.team_id = ?
    GROUP BY t.id, t.title
)
SELECT
    CAST('status' AS CHAR(32)) AS section,
    CAST(status AS CHAR(32)) AS label,
    CAST(NULL AS SIGNED) AS user_id,
    CAST(NULL AS CHAR(255)) AS user_name,
    CAST(NULL AS SIGNED) AS task_id,
    CAST(NULL AS CHAR(255)) AS task_title,
    CAST(task_count AS SIGNED) AS metric,
    CAST(NULL AS DECIMAL(30, 6)) AS avg_hours
FROM status_counts
UNION ALL
SELECT
    CAST('top_assignee' AS CHAR(32)),
    CAST(NULL AS CHAR(32)),
    CAST(assignee_id AS SIGNED),
    CAST(assignee_name AS CHAR(255)),
    CAST(NULL AS SIGNED),
    CAST(NULL AS CHAR(255)),
    CAST(closed_count AS SIGNED),
    CAST(NULL AS DECIMAL(30, 6))
FROM recent_closed
UNION ALL
SELECT
    CAST('avg_close_time' AS CHAR(32)),
    CAST(NULL AS CHAR(32)),
    CAST(NULL AS SIGNED),
    CAST(NULL AS CHAR(255)),
    CAST(NULL AS SIGNED),
    CAST(NULL AS CHAR(255)),
    CAST(NULL AS SIGNED),
    CAST(avg_hours AS DECIMAL(30, 6))
FROM close_time
UNION ALL
SELECT
    CAST('task_comments' AS CHAR(32)),
    CAST(NULL AS CHAR(32)),
    CAST(NULL AS SIGNED),
    CAST(NULL AS CHAR(255)),
    CAST(task_id AS SIGNED),
    CAST(title AS CHAR(255)),
    CAST(comment_count AS SIGNED),
    CAST(NULL AS DECIMAL(30, 6))
FROM comment_counts
`
