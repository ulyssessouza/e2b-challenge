package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"e2b-challenge/internal/db"
	"e2b-challenge/internal/pagination"

	"github.com/lib/pq"
)

type SandboxService struct {
	q                    *db.Queries
	maxRunningPerProject int
}

func NewSandboxService(q *db.Queries, maxRunningPerProject int) *SandboxService {
	return &SandboxService{q: q, maxRunningPerProject: maxRunningPerProject}
}

func (s *SandboxService) Create(ctx context.Context, projectID, userID, name, sandboxID string) (*db.Sandbox, bool, error) {
	if sandboxID != "" {
		sandbox, err := s.Restart(ctx, projectID, userID, sandboxID)
		return sandbox, false, err
	}

	if s.maxRunningPerProject > 0 {
		running, err := s.q.CountRunningSandboxesByProject(ctx, projectID)
		if err != nil {
			return nil, false, fmt.Errorf("counting running sandboxes: %w", err)
		}
		if running >= int64(s.maxRunningPerProject) {
			return nil, false, fmt.Errorf("%w: project already has %d running sandboxes", ErrQuotaExceeded, running)
		}
	}

	sandbox, err := s.q.CreateSandbox(ctx, db.CreateSandboxParams{
		ProjectID: projectID,
		UserID:    userID,
		Name:      name,
	})
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23503" {
			return nil, false, fmt.Errorf("%w: project", ErrNotFound)
		}
		return nil, false, fmt.Errorf("creating sandbox: %w", err)
	}
	return &sandbox, true, nil
}

func (s *SandboxService) Restart(ctx context.Context, projectID, userID, sandboxID string) (*db.Sandbox, error) {
	sandbox, err := s.q.GetSandboxByIDAndUser(ctx, db.GetSandboxByIDAndUserParams{
		ID:     sandboxID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: sandbox", ErrNotFound)
		}
		return nil, fmt.Errorf("getting sandbox: %w", err)
	}

	if sandbox.ProjectID != projectID {
		return nil, fmt.Errorf("%w: sandbox does not belong to this project", ErrNotFound)
	}

	if !sandbox.StoppedAt.Valid {
		return &sandbox, nil
	}

	// The guarded UPDATE re-checks membership and the stopped state at write
	// time, so a revocation between the read and the write cannot resurrect
	// the sandbox for a former member, and a concurrent restart converges to
	// a no-op instead of doubling.
	restarted, err := s.q.RestartSandbox(ctx, db.RestartSandboxParams{
		ID:     sandboxID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Lost the race: the sandbox is running again (someone else
			// restarted it) or it is gone. Re-read for a truthful answer.
			sandbox, err = s.q.GetSandboxByIDAndUser(ctx, db.GetSandboxByIDAndUserParams{
				ID:     sandboxID,
				UserID: userID,
			})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return nil, fmt.Errorf("%w: sandbox", ErrNotFound)
				}
				return nil, fmt.Errorf("reading sandbox: %w", err)
			}
			return &sandbox, nil
		}
		return nil, fmt.Errorf("restarting sandbox: %w", err)
	}
	return &restarted, nil
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
	if sandboxes == nil {
		sandboxes = []db.Sandbox{}
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
				return fmt.Errorf("%w: sandbox", ErrNotFound)
			}
			return fmt.Errorf("checking sandbox: %w", err)
		}
		if updated.StoppedAt.Valid {
			return fmt.Errorf("%w: sandbox already stopped", ErrConflict)
		}
		return fmt.Errorf("%w: sandbox was modified concurrently, try again", ErrConflict)
	}
	return nil
}
