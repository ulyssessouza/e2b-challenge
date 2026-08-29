package handler

import (
	"time"

	"e2b-challenge/internal/db"
)

// DTOs decouple the API contract from the database models: explicit
// snake_case field names and no sql.Null* leakage.

type ProjectDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type SandboxDTO struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	UserID    string     `json:"user_id"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"created_at"`
	StoppedAt *time.Time `json:"stopped_at"`
}

type UserDTO struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

func toProjectDTO(p db.Project) ProjectDTO {
	return ProjectDTO{ID: p.ID, Name: p.Name, CreatedAt: p.CreatedAt}
}

func toProjectDTOs(projects []db.Project) []ProjectDTO {
	dtos := make([]ProjectDTO, 0, len(projects))
	for _, p := range projects {
		dtos = append(dtos, toProjectDTO(p))
	}
	return dtos
}

func toSandboxDTO(s db.Sandbox) SandboxDTO {
	dto := SandboxDTO{
		ID:        s.ID,
		ProjectID: s.ProjectID,
		UserID:    s.UserID,
		Name:      s.Name,
		CreatedAt: s.CreatedAt,
	}
	if s.StoppedAt.Valid {
		t := s.StoppedAt.Time
		dto.StoppedAt = &t
	}
	return dto
}

func toSandboxDTOs(sandboxes []db.Sandbox) []SandboxDTO {
	dtos := make([]SandboxDTO, 0, len(sandboxes))
	for _, s := range sandboxes {
		dtos = append(dtos, toSandboxDTO(s))
	}
	return dtos
}

func toUserDTO(u db.User) UserDTO {
	return UserDTO{ID: u.ID, Email: u.Email, Name: u.Name}
}
