package store

import (
	"context"
	"database/sql"
	"time"

	"taskmanager/internal/model"
)

func (s *Store) CreateTeam(ctx context.Context, name string, ownerID int64) (*model.Team, error) {
	now := time.Now().UTC()
	var team model.Team

	err := s.WithTx(ctx, func(txCtx context.Context) error {
		result, err := s.q(txCtx).ExecContext(txCtx,
			`INSERT INTO teams (name, created_by, created_at) VALUES (?, ?, ?)`,
			name, ownerID, now,
		)
		if err != nil {
			return err
		}
		teamID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := s.q(txCtx).ExecContext(txCtx,
			`INSERT INTO team_members (team_id, user_id, role, created_at) VALUES (?, ?, 'owner', ?)`,
			teamID, ownerID, now,
		); err != nil {
			return err
		}
		team = model.Team{
			ID:          teamID,
			Name:        name,
			CreatedBy:   ownerID,
			Role:        model.RoleOwner,
			MemberCount: 1,
			CreatedAt:   now,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &team, nil
}

func (s *Store) ListTeams(ctx context.Context, userID int64) ([]model.Team, error) {
	rows, err := s.q(ctx).QueryContext(ctx, `
		SELECT t.id, t.name, t.created_by, t.created_at, me.role, COUNT(all_m.user_id) AS member_count
		FROM teams t
		INNER JOIN team_members me ON me.team_id = t.id AND me.user_id = ?
		LEFT JOIN team_members all_m ON all_m.team_id = t.id
		GROUP BY t.id, t.name, t.created_by, t.created_at, me.role
		ORDER BY t.created_at DESC, t.id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	teams := make([]model.Team, 0)
	for rows.Next() {
		var team model.Team
		if err := rows.Scan(&team.ID, &team.Name, &team.CreatedBy, &team.CreatedAt, &team.Role, &team.MemberCount); err != nil {
			return nil, err
		}
		teams = append(teams, team)
	}
	return teams, rows.Err()
}

func (s *Store) GetTeamMembership(ctx context.Context, teamID, userID int64) (*model.TeamMember, error) {
	return scanTeamMember(s.q(ctx).QueryRowContext(ctx, `
		SELECT tm.team_id, tm.user_id, tm.role, tm.created_at, u.name, u.email
		FROM team_members tm
		INNER JOIN users u ON u.id = tm.user_id
		WHERE tm.team_id = ? AND tm.user_id = ?
	`, teamID, userID))
}

func (s *Store) LockTeamMembership(ctx context.Context, teamID, userID int64) (*model.TeamMember, error) {
	return scanTeamMember(s.q(ctx).QueryRowContext(ctx, `
		SELECT tm.team_id, tm.user_id, tm.role, tm.created_at, u.name, u.email
		FROM team_members tm
		INNER JOIN users u ON u.id = tm.user_id
		WHERE tm.team_id = ? AND tm.user_id = ?
		FOR UPDATE
	`, teamID, userID))
}

func (s *Store) AddTeamMember(ctx context.Context, teamID, userID int64, role string) (*model.TeamMember, error) {
	now := time.Now().UTC()
	if _, err := s.q(ctx).ExecContext(ctx,
		`INSERT INTO team_members (team_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`,
		teamID, userID, role, now,
	); err != nil {
		if isDuplicateKey(err) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return s.GetTeamMembership(ctx, teamID, userID)
}

func (s *Store) UpdateTeamMemberRole(ctx context.Context, teamID, userID int64, role string) (*model.TeamMember, error) {
	result, err := s.q(ctx).ExecContext(ctx,
		`UPDATE team_members SET role = ? WHERE team_id = ? AND user_id = ?`,
		role, teamID, userID,
	)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		member, getErr := s.GetTeamMembership(ctx, teamID, userID)
		if getErr != nil {
			return nil, getErr
		}
		return member, nil
	}
	return s.GetTeamMembership(ctx, teamID, userID)
}

func (s *Store) ListTeamMembers(ctx context.Context, teamID int64) ([]model.TeamMember, error) {
	rows, err := s.q(ctx).QueryContext(ctx, `
		SELECT tm.team_id, tm.user_id, tm.role, tm.created_at, u.name, u.email
		FROM team_members tm
		INNER JOIN users u ON u.id = tm.user_id
		WHERE tm.team_id = ?
		ORDER BY FIELD(tm.role, 'owner', 'admin', 'member'), u.name, tm.user_id
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := make([]model.TeamMember, 0)
	for rows.Next() {
		member, err := scanTeamMemberRow(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, *member)
	}
	return members, rows.Err()
}

func (s *Store) IsTeamMember(ctx context.Context, teamID, userID int64) (bool, error) {
	var exists bool
	err := s.q(ctx).QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM team_members WHERE team_id = ? AND user_id = ?)`,
		teamID, userID,
	).Scan(&exists)
	return exists, err
}

func scanTeamMember(row rowScanner) (*model.TeamMember, error) {
	member, err := scanTeamMemberRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return member, nil
}

func scanTeamMemberRow(row rowScanner) (*model.TeamMember, error) {
	var member model.TeamMember
	if err := row.Scan(&member.TeamID, &member.UserID, &member.Role, &member.JoinedAt, &member.Name, &member.Email); err != nil {
		return nil, err
	}
	return &member, nil
}
