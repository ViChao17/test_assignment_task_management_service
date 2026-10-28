package service

import (
	"context"
	"database/sql"
	"errors"

	"taskmanager/internal/app_errors"
	"taskmanager/internal/model"
	"taskmanager/internal/store"
)

type TeamService struct {
	store *store.Store
}

func NewTeamService(s *store.Store) *TeamService {
	return &TeamService{store: s}
}

func (s *TeamService) Create(ctx context.Context, userID int64, name string) (*model.Team, error) {
	name, err := validateTeamName(name)
	if err != nil {
		return nil, err
	}
	team, err := s.store.CreateTeam(ctx, name, userID)
	if err != nil {
		return nil, app_errors.Internal(err)
	}
	return team, nil
}

func (s *TeamService) List(ctx context.Context, userID int64) ([]model.Team, error) {
	teams, err := s.store.ListTeams(ctx, userID)
	if err != nil {
		return nil, app_errors.Internal(err)
	}
	return teams, nil
}

func (s *TeamService) ListMembers(ctx context.Context, userID, teamID int64) ([]model.TeamMember, error) {
	if _, err := s.store.GetTeamMembership(ctx, teamID, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, app_errors.NotFound("team not found")
		}
		return nil, app_errors.Internal(err)
	}
	members, err := s.store.ListTeamMembers(ctx, teamID)
	if err != nil {
		return nil, app_errors.Internal(err)
	}
	return members, nil
}

type InviteInput struct {
	UserID int64
	Role   string
}

func (s *TeamService) Invite(ctx context.Context, actorID, teamID int64, input InviteInput) (*model.TeamMember, error) {
	if input.UserID <= 0 {
		return nil, app_errors.BadRequest("user_id must be positive")
	}
	if input.Role == "" {
		input.Role = model.RoleMember
	}
	if input.Role == model.RoleOwner {
		return nil, app_errors.BadRequest("the owner role cannot be assigned through an invitation")
	}
	if input.Role != model.RoleAdmin && input.Role != model.RoleMember {
		return nil, app_errors.BadRequest("role must be admin or member")
	}

	var member *model.TeamMember
	err := s.store.WithTx(ctx, func(txCtx context.Context) error {
		actor, err := s.store.LockTeamMembership(txCtx, teamID, actorID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return app_errors.NotFound("team not found")
			}
			return err
		}
		if !CanInvite(actor.Role, input.Role) {
			return app_errors.Forbidden("you are not allowed to invite a user with this role")
		}

		if _, err := s.store.GetUserByID(txCtx, input.UserID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return app_errors.NotFound("user not found")
			}
			return err
		}

		member, err = s.store.AddTeamMember(txCtx, teamID, input.UserID, input.Role)
		if errors.Is(err, store.ErrDuplicate) {
			return app_errors.Conflict("already_member", "user is already a member of this team")
		}
		return err
	})
	if err != nil {
		var appErr *app_errors.Error
		if errors.As(err, &appErr) {
			return nil, err
		}
		return nil, app_errors.Internal(err)
	}
	return member, nil
}

func (s *TeamService) UpdateMemberRole(ctx context.Context, actorID, teamID, targetUserID int64, role string) (*model.TeamMember, error) {
	if !model.IsValidRole(role) {
		return nil, app_errors.BadRequest("role must be one of owner, admin, member")
	}
	if role == model.RoleOwner {
		return nil, app_errors.BadRequest("owner role transfer is not supported")
	}

	var member *model.TeamMember
	err := s.store.WithTx(ctx, func(txCtx context.Context) error {
		actor, err := s.store.LockTeamMembership(txCtx, teamID, actorID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return app_errors.NotFound("team not found")
			}
			return err
		}
		if !CanManageMemberRoles(actor.Role) {
			return app_errors.Forbidden("only the team owner can change member roles")
		}

		target, err := s.store.LockTeamMembership(txCtx, teamID, targetUserID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return app_errors.NotFound("team member not found")
			}
			return err
		}
		if target.Role == model.RoleOwner {
			return app_errors.Forbidden("the team owner role cannot be changed")
		}

		member, err = s.store.UpdateTeamMemberRole(txCtx, teamID, targetUserID, role)
		return err
	})
	if err != nil {
		var appErr *app_errors.Error
		if errors.As(err, &appErr) {
			return nil, err
		}
		return nil, app_errors.Internal(err)
	}
	return member, nil
}

