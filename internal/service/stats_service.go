package service

import (
	"context"
	"database/sql"
	"errors"

	"taskmanager/internal/app_errors"
	"taskmanager/internal/model"
	"taskmanager/internal/store"
)

type StatsService struct {
	store *store.Store
}

func NewStatsService(s *store.Store) *StatsService {
	return &StatsService{store: s}
}

func (s *StatsService) TeamStats(ctx context.Context, actorID, teamID int64) (*model.TeamStats, error) {
	member, err := s.store.GetTeamMembership(ctx, teamID, actorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, app_errors.NotFound("team not found")
		}
		return nil, app_errors.Internal(err)
	}
	if !CanViewStats(member.Role) {
		return nil, app_errors.Forbidden("only the team owner or an administrator can view analytics")
	}
	stats, err := s.store.TeamStats(ctx, teamID)
	if err != nil {
		return nil, app_errors.Internal(err)
	}
	return stats, nil
}

