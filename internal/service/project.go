package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"e2b-challenge/internal/db"
	"e2b-challenge/internal/pagination"
	"go.opentelemetry.io/otel/attribute"
)

type ProjectService struct {
	q *db.Queries
}

func NewProjectService(q *db.Queries) *ProjectService {
	return &ProjectService{q: q}
}

func (s *ProjectService) Create(ctx context.Context, name, ownerID string) (*db.Project, error) {
	ctx, span := tracer.Start(ctx, "project.Create")
	defer span.End()
	span.SetAttributes(
		attribute.String("project.name", name),
		attribute.String("project.owner_id", ownerID),
	)

	project, err := s.q.CreateProject(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}

	if err := s.q.AddProjectMember(ctx, db.AddProjectMemberParams{
		ProjectID: project.ID,
		UserID:    ownerID,
		Role:      "owner",
	}); err != nil {
		return nil, fmt.Errorf("adding owner: %w", err)
	}

	span.SetAttributes(attribute.String("project.id", project.ID))
	return &project, nil
}

func (s *ProjectService) ListByUser(ctx context.Context, userID string, p pagination.Params) ([]db.Project, int64, error) {
	ctx, span := tracer.Start(ctx, "project.ListByUser")
	defer span.End()
	span.SetAttributes(
		attribute.String("project.user_id", userID),
		attribute.Int("pagination.limit", int(p.Limit)),
		attribute.Int("pagination.offset", int(p.Offset)),
	)

	projects, err := s.q.ListProjectsByUser(ctx, db.ListProjectsByUserParams{
		UserID: userID,
		Limit:  p.Limit,
		Offset: p.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing projects: %w", err)
	}

	total, err := s.q.CountProjectsByUser(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting projects: %w", err)
	}

	span.SetAttributes(attribute.Int64("project.total", total))
	return projects, total, nil
}

func (s *ProjectService) GetByID(ctx context.Context, id string) (*db.Project, error) {
	ctx, span := tracer.Start(ctx, "project.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("project.id", id))

	project, err := s.q.GetProjectByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting project: %w", err)
	}
	return &project, nil
}

func (s *ProjectService) AddMember(ctx context.Context, projectID, userEmail, role string) (*db.User, error) {
	ctx, span := tracer.Start(ctx, "project.AddMember")
	defer span.End()
	span.SetAttributes(
		attribute.String("project.id", projectID),
		attribute.String("project.member_email", userEmail),
		attribute.String("project.member_role", role),
	)

	user, err := s.q.GetUserByEmail(ctx, userEmail)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("user not found: %s", userEmail)
		}
		return nil, fmt.Errorf("looking up user: %w", err)
	}

	if err := s.q.AddProjectMember(ctx, db.AddProjectMemberParams{
		ProjectID: projectID,
		UserID:    user.ID,
		Role:      role,
	}); err != nil {
		return nil, fmt.Errorf("adding member: %w", err)
	}

	span.SetAttributes(attribute.String("project.member_id", user.ID))
	return &user, nil
}