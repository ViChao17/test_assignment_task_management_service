package service

import (
	"testing"

	"taskmanager/internal/model"
)

func int64Ptr(value int64) *int64 { return &value }

func stringPtr(value string) *string { return &value }

func TestCanEditTask(t *testing.T) {
	task := &model.Task{
		CreatedBy:   10,
		AssigneeID:  int64Ptr(20),
		Title:       "Original",
		Description: "Original description",
		Status:      model.StatusTodo,
	}

	tests := []struct {
		name     string
		role     string
		actorID  int64
		mutation TaskMutation
		want     bool
	}{
		{
			name:     "owner may edit anything",
			role:     model.RoleOwner,
			actorID:  99,
			mutation: TaskMutation{Title: stringPtr("changed"), AssigneeID: int64Ptr(30), AssigneeSet: true},
			want:     true,
		},
		{
			name:     "admin may edit anything",
			role:     model.RoleAdmin,
			actorID:  99,
			mutation: TaskMutation{Status: stringPtr(model.StatusDone)},
			want:     true,
		},
		{
			name:     "creator may edit title",
			role:     model.RoleMember,
			actorID:  10,
			mutation: TaskMutation{Title: stringPtr("changed")},
			want:     true,
		},
		{
			name:     "creator may unassign",
			role:     model.RoleMember,
			actorID:  10,
			mutation: TaskMutation{AssigneeSet: true},
			want:     true,
		},
		{
			name:     "assignee may change status",
			role:     model.RoleMember,
			actorID:  20,
			mutation: TaskMutation{Status: stringPtr(model.StatusDone)},
			want:     true,
		},
		{
			name:     "assignee cannot change title",
			role:     model.RoleMember,
			actorID:  20,
			mutation: TaskMutation{Title: stringPtr("changed")},
			want:     false,
		},
		{
			name:     "assignee cannot change description",
			role:     model.RoleMember,
			actorID:  20,
			mutation: TaskMutation{Description: stringPtr("changed")},
			want:     false,
		},
		{
			name:     "assignee cannot change assignee",
			role:     model.RoleMember,
			actorID:  20,
			mutation: TaskMutation{AssigneeID: int64Ptr(30), AssigneeSet: true},
			want:     false,
		},
		{
			name:     "assignee cannot unassign",
			role:     model.RoleMember,
			actorID:  20,
			mutation: TaskMutation{AssigneeSet: true},
			want:     false,
		},
		{
			name:     "assignee cannot change status and unassign",
			role:     model.RoleMember,
			actorID:  20,
			mutation: TaskMutation{Status: stringPtr(model.StatusDone), AssigneeSet: true},
			want:     false,
		},
		{
			name:     "unrelated member cannot edit",
			role:     model.RoleMember,
			actorID:  30,
			mutation: TaskMutation{Status: stringPtr(model.StatusDone)},
			want:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := CanEditTask(test.role, test.actorID, task, test.mutation)
			if got != test.want {
				t.Fatalf("CanEditTask() = %v, want %v", got, test.want)
			}
		})
	}
}
