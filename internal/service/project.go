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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.q.WithTx(tx)

	project, err := qtx.CreateProject(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}

	if err := qtx.AddProjectMember(ctx, db.AddProjectMemberParams{
		ProjectID: project.ID,
		UserID:    ownerID,
		Role:      "owner",
	}); err != nil {
		return nil, fmt.Errorf("adding owner: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing project: %w", err)
	}

	return &project, nil
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

func (s *ProjectService) AddMember(ctx context.Context, projectID, userEmail, role string) (*db.User, error) {
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

	if err := qtx.AddProjectMember(ctx, db.AddProjectMemberParams{
		ProjectID: projectID,
		UserID:    user.ID,
		Role:      role,
	}); err != nil {
		var pqErr *pq.Error
		switch {
		case errors.As(err, &pqErr) && pqErr.Code == "23505":
			return nil, fmt.Errorf("%w: user is already a member of this project", ErrConflict)
		case errors.As(err, &pqErr) && pqErr.Code == "23503":
			return nil, fmt.Errorf("%w: project", ErrNotFound)
		}
		return nil, fmt.Errorf("adding member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing membership: %w", err)
	}

	return &user, nil
}
