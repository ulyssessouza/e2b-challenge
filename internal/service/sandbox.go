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

func (s *SandboxService) Create(ctx context.Context, projectID, userID string) (*db.Sandbox, error) {
	sandbox, err := s.q.CreateSandbox(ctx, db.CreateSandboxParams{
		ProjectID: projectID,
		UserID:    userID,
	})
	if err != nil {
		return nil, fmt.Errorf("creating sandbox: %w", err)
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
	sandbox, err := s.q.GetSandboxByID(ctx, sandboxID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("sandbox not found")
		}
		return fmt.Errorf("getting sandbox: %w", err)
	}

	_, err = s.q.GetProjectMember(ctx, db.GetProjectMemberParams{
		ProjectID: sandbox.ProjectID,
		UserID:    userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("not a member of this sandbox's project")
		}
		return fmt.Errorf("checking membership: %w", err)
	}

	if sandbox.Status == "stopped" {
		return fmt.Errorf("sandbox already stopped")
	}

	rows, err := s.q.UpdateSandboxStatus(ctx, db.UpdateSandboxStatusParams{
		ID:      sandboxID,
		Status:  "stopped",
		Version: sandbox.Version,
	})
	if err != nil {
		return fmt.Errorf("stopping sandbox: %w", err)
	}

	if rows == 0 {
		// Version mismatch — concurrent modification
		// Re-read to give the client useful feedback
		updated, err := s.q.GetSandboxByID(ctx, sandboxID)
		if err != nil {
			return fmt.Errorf("conflict stopping sandbox: %w", err)
		}
		if updated.Status == "stopped" {
			return fmt.Errorf("sandbox already stopped")
		}
		return fmt.Errorf("sandbox was modified concurrently, try again")
	}

	return nil
}