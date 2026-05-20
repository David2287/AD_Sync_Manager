package interfaces

import (
	"context"

	"ad-sync-manager/internal/domain/entity"
)

// NoteService is the port for the markdown note use-case.
type NoteService interface {
	Create(ctx context.Context, employeeID, authorID, markdown string) (*entity.Note, error)
	Update(ctx context.Context, noteID, authorID, markdown string) (*entity.Note, error)
	ListForEmployee(ctx context.Context, employeeID string) ([]*entity.Note, error)
	Delete(ctx context.Context, noteID, requestorID string) error
}
