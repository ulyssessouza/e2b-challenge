package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"e2b-challenge/internal/db"
	"e2b-challenge/internal/pagination"
)

type SandboxService struct {
	q *db.Queries
}

func NewSandboxService(q *db.Queries) *SandboxService {
	return &SandboxService{q: q}
}

func (s *SandboxService) Create(ctx context.Context, projectID, userID, name, sandboxID string) (*db.Sandbox, error) {
	if sandboxID != "" {
		return s.Restart(ctx, projectID, userID, sandboxID)
	}

	sandbox, err := s.q.CreateSandbox(ctx, db.CreateSandboxParams{
		ProjectID: projectID,
		UserID:    userID,
		Name:      name,
	})
	if err != nil {
		return nil, fmt.Errorf("creating sandbox: %w", err)
	}
	return &sandbox, nil
}

func (s *SandboxService) Restart(ctx context.Context, projectID, userID, sandboxID string) (*db.Sandbox, error) {
	sandbox, err := s.q.GetSandboxByIDAndUser(ctx, db.GetSandboxByIDAndUserParams{
		ID:     sandboxID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("sandbox not found")
		}
		return nil, fmt.Errorf("getting sandbox: %w", err)
	}

	if sandbox.ProjectID != projectID {
		return nil, fmt.Errorf("sandbox not found in this project")
	}

	if !sandbox.StoppedAt.Valid {
		return &sandbox, nil
	}

	rows, err := s.q.RestartSandbox(ctx, db.RestartSandboxParams{
		ID:      sandboxID,
		Version: sandbox.Version,
	})
	if err != nil {
		return nil, fmt.Errorf("restarting sandbox: %w", err)
	}
	if rows == 0 {
		return nil, fmt.Errorf("sandbox was modified concurrently, try again")
	}

	sandbox, err = s.q.GetSandboxByIDAndUser(ctx, db.GetSandboxByIDAndUserParams{
		ID:     sandboxID,
		UserID: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("reading restarted sandbox: %w", err)
	}
	return &sandbox, nil
}

func (s *SandboxService) ListByProject(ctx context.Context, projectID string, p pagination.Params) ([]db.Sandbox, int64, error) {
	sandboxes, err := s.q.ListSandboxesByProject(ctx, db.ListSandboxesByProjectParams{
		ProjectID: projectID,
		Limit:     p.Limit,
		Offset:    p.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing sandboxes: %w", err)
	}

	total, err := s.q.CountSandboxesByProject(ctx, projectID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting sandboxes: %w", err)
	}

	return sandboxes, total, nil
}

func (s *SandboxService) Stop(ctx context.Context, sandboxID, userID string) error {
	rows, err := s.q.StopSandbox(ctx, db.StopSandboxParams{
		ID:     sandboxID,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("stopping sandbox: %w", err)
	}

	if rows == 0 {
		updated, err := s.q.GetSandboxByIDAndUser(ctx, db.GetSandboxByIDAndUserParams{
			ID:     sandboxID,
			UserID: userID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("sandbox not found")
			}
			return fmt.Errorf("checking sandbox: %w", err)
		}
		if updated.StoppedAt.Valid {
			return fmt.Errorf("sandbox already stopped")
		}
		return fmt.Errorf("sandbox was modified concurrently, try again")
	}

	return nil
}