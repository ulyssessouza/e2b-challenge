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

type ProjectService struct {
	q  *db.Queries
	db *sql.DB
}

func NewProjectService(q *db.Queries, db *sql.DB) *ProjectService {
	return &ProjectService{q: q, db: db}
}

func (s *ProjectService) Create(ctx context.Context, name, ownerID string) (*db.Project, error) {
	plan, owned, err := s.planUsage(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	if plan.MaxProjects > 0 && owned >= int64(plan.MaxProjects) {
		return nil, fmt.Errorf("%w: plan %q allows %d owned projects", ErrQuotaExceeded, plan.Name, plan.MaxProjects)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.q.WithTx(tx)

	project, err := qtx.CreateProject(ctx, db.CreateProjectParams{
		OwnerID: ownerID,
		Name:    name,
	})
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == errCodeUniqueViolation {
			return nil, fmt.Errorf("%w: you already have a project named %q", ErrConflict, name)
		}
		return nil, fmt.Errorf("creating project: %w", err)
	}

	// The owner is a member too — project_users is the access list,
	// projects.owner_id carries the ownership.
	if err := qtx.CreateProjectMember(ctx, db.CreateProjectMemberParams{
		ProjectID: project.ID,
		UserID:    ownerID,
	}); err != nil {
		return nil, fmt.Errorf("adding owner: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing project: %w", err)
	}

	return &project, nil
}

// planUsage returns the owner's plan and how many projects they currently
// own. Enforcement is intentionally check-then-create: concurrent creates
// may overshoot by a few, bounded by request rate. For unlimited plans the
// count is skipped entirely.
func (s *ProjectService) planUsage(ctx context.Context, ownerID string) (db.Plan, int64, error) {
	plan, err := s.q.GetUserPlan(ctx, ownerID)
	if err != nil {
		return db.Plan{}, 0, fmt.Errorf("getting plan: %w", err)
	}
	if plan.MaxProjects <= 0 {
		return plan, 0, nil // unlimited
	}
	owned, err := s.q.CountProjectsByOwner(ctx, db.CountProjectsByOwnerParams{
		OwnerID: ownerID,
		Limit:   plan.MaxProjects,
	})
	if err != nil {
		return db.Plan{}, 0, fmt.Errorf("counting owned projects: %w", err)
	}
	return plan, owned, nil
}

func (s *ProjectService) ListByUser(ctx context.Context, userID string, p pagination.Params) ([]db.Project, int64, error) {
	projects, err := s.q.ListProjectsByUser(ctx, db.ListProjectsByUserParams{
		UserID: userID,
		Limit:  p.Limit,
		Offset: p.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing projects: %w", err)
	}
	if projects == nil {
		projects = []db.Project{}
	}

	total, err := s.q.CountProjectsByUser(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting projects: %w", err)
	}

	return projects, total, nil
}

func (s *ProjectService) GetByID(ctx context.Context, id string) (*db.Project, error) {
	project, err := s.q.GetProjectByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: project", ErrNotFound)
		}
		return nil, fmt.Errorf("getting project: %w", err)
	}
	return &project, nil
}

func (s *ProjectService) AddMember(ctx context.Context, projectID, userEmail string) (*db.User, error) {
	// Transaction so the user cannot be deleted between the lookup and the
	// membership insert (which would surface as a raw FK violation).
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.q.WithTx(tx)

	user, err := qtx.GetUserByEmail(ctx, userEmail)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: user %s", ErrNotFound, userEmail)
		}
		return nil, fmt.Errorf("looking up user: %w", err)
	}

	if err := qtx.CreateProjectMember(ctx, db.CreateProjectMemberParams{
		ProjectID: projectID,
		UserID:    user.ID,
	}); err != nil {
		var pqErr *pq.Error
		switch {
		case errors.As(err, &pqErr) && pqErr.Code == errCodeUniqueViolation:
			return nil, fmt.Errorf("%w: user is already a member of this project", ErrConflict)
		case errors.As(err, &pqErr) && pqErr.Code == errCodeForeignKeyViolation:
			return nil, fmt.Errorf("%w: project", ErrNotFound)
		}
		return nil, fmt.Errorf("adding member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing membership: %w", err)
	}

	return &user, nil
}
