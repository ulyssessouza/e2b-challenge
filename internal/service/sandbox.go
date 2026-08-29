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
	q *db.Queries
}

func NewSandboxService(q *db.Queries) *SandboxService {
	return &SandboxService{q: q}
}

func (s *SandboxService) Create(ctx context.Context, projectID, userID, name, sandboxID string) (*db.Sandbox, bool, error) {
	if sandboxID != "" {
		sandbox, err := s.Restart(ctx, projectID, userID, sandboxID)
		return sandbox, false, err
	}

	if err := s.checkRunningQuota(ctx, userID); err != nil {
		return nil, false, err
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

// checkRunningQuota rejects the action when the user's plan cap on running
// sandboxes is exhausted. Applied to BOTH create and restart: while a sandbox
// is stopped it does not count toward the quota, so restarting one is growth
// whenever new sandboxes were created since the stop.
func (s *SandboxService) checkRunningQuota(ctx context.Context, userID string) error {
	plan, err := s.q.GetUserPlan(ctx, userID)
	if err != nil {
		return fmt.Errorf("getting plan: %w", err)
	}
	running, err := s.q.CountRunningSandboxesByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("counting running sandboxes: %w", err)
	}
	if plan.MaxRunningSandboxes > 0 && running >= int64(plan.MaxRunningSandboxes) {
		return fmt.Errorf("%w: plan %q allows %d running sandboxes", ErrQuotaExceeded, plan.Name, plan.MaxRunningSandboxes)
	}
	return nil
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

	if err := s.checkRunningQuota(ctx, userID); err != nil {
		return nil, err
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
