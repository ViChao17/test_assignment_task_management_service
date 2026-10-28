package service

import "taskmanager/internal/model"

// TaskMutation описывает поля, которые клиент запросил изменить.
//
// AssigneeSet отличает отсутствие assignee_id от явного null: true означает,
// что поле было в запросе, даже если AssigneeID == nil (снять исполнителя).
type TaskMutation struct {
	Title       *string
	Description *string
	Status      *string
	AssigneeID  *int64
	AssigneeSet bool
}

// CanEditTask применяет правила редактирования:
//   - owner/admin может редактировать любую задачу команды;
//   - создатель задачи может менять все изменяемые поля;
//   - исполнитель может менять только статус (и не может переназначать задачу);
//   - остальным запрещено.
func CanEditTask(role string, actorID int64, task *model.Task, mutation TaskMutation) bool {
	if role == model.RoleOwner || role == model.RoleAdmin {
		return true
	}
	if task.CreatedBy == actorID {
		return true
	}
	if task.AssigneeID == nil || *task.AssigneeID != actorID {
		return false
	}

	if mutation.Title != nil && *mutation.Title != task.Title {
		return false
	}
	if mutation.Description != nil && *mutation.Description != task.Description {
		return false
	}
	if mutation.Title != nil || mutation.Description != nil || mutation.AssigneeSet || mutation.AssigneeID != nil {
		return false
	}
	return true
}

// CanInvite ограничивает назначение ролей: owner может пригласить admin или
// member, admin — только member. Роль owner через приглашение не выдаётся.
func CanInvite(actorRole, requestedRole string) bool {
	switch actorRole {
	case model.RoleOwner:
		return requestedRole == model.RoleAdmin || requestedRole == model.RoleMember
	case model.RoleAdmin:
		return requestedRole == model.RoleMember
	default:
		return false
	}
}

func CanManageMemberRoles(actorRole string) bool {
	return actorRole == model.RoleOwner
}

func CanViewStats(role string) bool {
	return role == model.RoleOwner || role == model.RoleAdmin
}
