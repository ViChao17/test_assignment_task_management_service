package integration

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"taskmanager/internal/app_errors"
	"taskmanager/internal/db"
	"taskmanager/internal/model"
	"taskmanager/internal/service"
	"taskmanager/internal/store"
)

// TestTeamStatsSQL — интеграционный тест SQL-отчёта.
//
//	TEST_MYSQL_DSN='root:password@tcp(127.0.0.1:3306)/taskmanager_test?parseTime=true' go test ./integration -run TestTeamStatsSQL -v
//
// Тест создаёт свои данные, проверяет доступ и удаляет всё после себя.
func TestTeamStatsSQL(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("TEST_MYSQL_DSN is not set; skipping MySQL integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	database, err := db.Open(ctx, dsn, 15*time.Second)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Logf("close database: %v", err)
		}
	})
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	st := store.New(database)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var userIDs []int64
	var teamIDs []int64
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if len(teamIDs) > 0 {
			marks := placeholders(len(teamIDs))
			queries := []string{
				"DELETE FROM task_comments WHERE task_id IN (SELECT id FROM tasks WHERE team_id IN (" + marks + "))",
				"DELETE FROM task_history WHERE task_id IN (SELECT id FROM tasks WHERE team_id IN (" + marks + "))",
				"DELETE FROM tasks WHERE team_id IN (" + marks + ")",
				"DELETE FROM team_members WHERE team_id IN (" + marks + ")",
				"DELETE FROM teams WHERE id IN (" + marks + ")",
			}
			for _, query := range queries {
				args := make([]any, len(teamIDs))
				for i, id := range teamIDs {
					args[i] = id
				}
				if _, err := database.ExecContext(cleanupCtx, query, args...); err != nil {
					t.Logf("cleanup query failed: %v", err)
				}
			}
		}
		if len(userIDs) > 0 {
			marks := placeholders(len(userIDs))
			args := make([]any, len(userIDs))
			for i, id := range userIDs {
				args[i] = id
			}
			if _, err := database.ExecContext(cleanupCtx, "DELETE FROM users WHERE id IN ("+marks+")", args...); err != nil {
				t.Logf("cleanup users failed: %v", err)
			}
		}
	})

	newUser := func(label string) *model.User {
		user, err := st.CreateUser(ctx, fmt.Sprintf("stats-%s-%s@example.test", suffix, label), "unused", label)
		if err != nil {
			t.Fatalf("create user %s: %v", label, err)
		}
		userIDs = append(userIDs, user.ID)
		return user
	}

	owner := newUser("owner")
	admin := newUser("admin")
	member := newUser("member")
	assigneeA := newUser("assignee-a")
	assigneeB := newUser("assignee-b")
	assigneeC := newUser("assignee-c")
	assigneeD := newUser("assignee-d")
	outsider := newUser("outsider")

	team, err := st.CreateTeam(ctx, "Stats team "+suffix, owner.ID)
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	teamIDs = append(teamIDs, team.ID)
	for _, memberInput := range []struct {
		user *model.User
		role string
	}{
		{admin, model.RoleAdmin},
		{member, model.RoleMember},
		{assigneeA, model.RoleMember},
		{assigneeB, model.RoleMember},
		{assigneeC, model.RoleMember},
		{assigneeD, model.RoleMember},
	} {
		if _, err := st.AddTeamMember(ctx, team.ID, memberInput.user.ID, memberInput.role); err != nil {
			t.Fatalf("add team member: %v", err)
		}
	}

	foreignTeam, err := st.CreateTeam(ctx, "Foreign team "+suffix, outsider.ID)
	if err != nil {
		t.Fatalf("create foreign team: %v", err)
	}
	teamIDs = append(teamIDs, foreignTeam.ID)

	now := time.Now().UTC()
	insertTask := func(teamID int64, title, status string, creator int64, assignee *int64, created, closed time.Time) *model.Task {
		task := &model.Task{
			TeamID:      teamID,
			Title:       title,
			Description: "",
			Status:      status,
			CreatedBy:   creator,
			AssigneeID:  assignee,
			CreatedAt:   created,
			UpdatedAt:   created,
			Version:     1,
		}
		if status == model.StatusDone {
			closedCopy := closed
			task.ClosedAt = &closedCopy
		}
		if err := st.InsertTask(ctx, task); err != nil {
			t.Fatalf("insert task %s: %v", title, err)
		}
		return task
	}

	doneA1 := insertTask(team.ID, "A closed inside 30d #1", model.StatusDone, owner.ID, &assigneeA.ID, now.Add(-14*time.Hour), now.Add(-2*time.Hour))
	insertTask(team.ID, "A closed inside 30d #2", model.StatusDone, member.ID, &assigneeA.ID, now.Add(-11*time.Hour), now.Add(-3*time.Hour))
	doneB := insertTask(team.ID, "B closed inside 30d", model.StatusDone, owner.ID, &assigneeB.ID, now.Add(-10*time.Hour), now.Add(-5*time.Hour))
	insertTask(team.ID, "C closed inside 30d", model.StatusDone, admin.ID, &assigneeC.ID, now.Add(-30*time.Hour), now.Add(-10*time.Hour))
	insertTask(team.ID, "D closed outside 30d", model.StatusDone, owner.ID, &assigneeD.ID, now.Add(-40*24*time.Hour-50*time.Hour), now.Add(-40*24*time.Hour))
	todo := insertTask(team.ID, "Todo without assignee", model.StatusTodo, owner.ID, nil, now.Add(-time.Hour), time.Time{})
	insertTask(team.ID, "Todo assigned to B", model.StatusTodo, admin.ID, &assigneeB.ID, now.Add(-2*time.Hour), time.Time{})
	insertTask(team.ID, "In progress", model.StatusInProgress, member.ID, &assigneeA.ID, now.Add(-3*time.Hour), time.Time{})

	// Задачи другой команды не должны попадать в отчёт.
	foreignDone := insertTask(foreignTeam.ID, "Foreign done", model.StatusDone, outsider.ID, &outsider.ID, now.Add(-5*time.Hour), now.Add(-1*time.Hour))
	insertTask(foreignTeam.ID, "Foreign todo", model.StatusTodo, outsider.ID, nil, now.Add(-1*time.Hour), time.Time{})

	addComment := func(taskID, userID int64, content string) {
		comment := &model.Comment{TaskID: taskID, UserID: userID, Content: content, CreatedAt: now}
		if err := st.InsertComment(ctx, comment); err != nil {
			t.Fatalf("insert comment: %v", err)
		}
	}
	addComment(doneA1.ID, member.ID, "first")
	addComment(doneA1.ID, admin.ID, "second")
	addComment(todo.ID, owner.ID, "todo comment")
	addComment(doneB.ID, owner.ID, "b comment")

	statsService := service.NewStatsService(st)
	stats, err := statsService.TeamStats(ctx, owner.ID, team.ID)
	if err != nil {
		t.Fatalf("owner stats: %v", err)
	}

	if stats.TeamID != team.ID {
		t.Fatalf("team_id = %d, want %d", stats.TeamID, team.ID)
	}
	if stats.TotalTasks != 8 {
		t.Fatalf("total_tasks = %d, want 8", stats.TotalTasks)
	}
	wantStatuses := map[string]int64{
		model.StatusTodo:       2,
		model.StatusInProgress: 1,
		model.StatusDone:       5,
	}
	for status, want := range wantStatuses {
		if got := stats.TasksByStatus[status]; got != want {
			t.Errorf("tasks_by_status[%s] = %d, want %d", status, got, want)
		}
	}

	if len(stats.TopAssignees30d) != 3 {
		t.Fatalf("top_assignees_30d length = %d, want 3: %+v", len(stats.TopAssignees30d), stats.TopAssignees30d)
	}
	wantTop := []struct {
		userID int64
		count  int64
	}{
		{assigneeA.ID, 2},
		{assigneeB.ID, 1},
		{assigneeC.ID, 1},
	}
	for i, want := range wantTop {
		got := stats.TopAssignees30d[i]
		if got.UserID != want.userID || got.ClosedTasks30d != want.count {
			t.Errorf("top_assignees_30d[%d] = (%d, %d), want (%d, %d)", i, got.UserID, got.ClosedTasks30d, want.userID, want.count)
		}
	}
	for _, assignee := range stats.TopAssignees30d {
		if assignee.UserID == assigneeD.ID {
			t.Fatal("assignee with only a 40-day-old task must not be in the 30-day top")
		}
	}

	if stats.AverageCloseTimeHours == nil {
		t.Fatal("average_close_time_hours must not be nil")
	}
	if math.Abs(*stats.AverageCloseTimeHours-19.0) > 0.01 {
		t.Fatalf("average_close_time_hours = %.4f, want 19.0", *stats.AverageCloseTimeHours)
	}

	commentCounts := map[int64]int64{}
	for _, item := range stats.CommentsByTask {
		commentCounts[item.TaskID] = item.CommentCount
	}
	if len(commentCounts) != 8 {
		t.Fatalf("comments_by_task covers %d tasks, want 8", len(commentCounts))
	}
	wantComments := map[int64]int64{
		doneA1.ID: 2,
		doneB.ID:  1,
		todo.ID:   1,
	}
	for taskID, want := range wantComments {
		if got := commentCounts[taskID]; got != want {
			t.Errorf("comments for task %d = %d, want %d", taskID, got, want)
		}
	}
	if got := commentCounts[foreignDone.ID]; got != 0 {
		t.Errorf("foreign team task leaked into comments_by_task: %d", got)
	}

	if _, err := statsService.TeamStats(ctx, admin.ID, team.ID); err != nil {
		t.Fatalf("admin must be able to read stats: %v", err)
	}
	if _, err := statsService.TeamStats(ctx, member.ID, team.ID); err == nil {
		t.Fatal("member must not be able to read stats")
	} else {
		assertStatus(t, err, 403)
	}
	if _, err := statsService.TeamStats(ctx, outsider.ID, team.ID); err == nil {
		t.Fatal("outsider must not be able to read stats")
	} else {
		assertStatus(t, err, 404)
	}
}

func assertStatus(t *testing.T, err error, want int) {
	t.Helper()
	var appErr *app_errors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error %v is not an application error", err)
	}
	if appErr.Status != want {
		t.Fatalf("error status = %d, want %d (%v)", appErr.Status, want, err)
	}
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}
