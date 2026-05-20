package interfaces

import (
	"context"

	"ad-sync-manager/internal/domain/entity"
)

// NoteRepository is the port for markdown note persistence.
// Stage 1 uses flat files under ./data/notes/; Stage 2 may migrate to DB.
type NoteRepository interface {
	Save(ctx context.Context, note *entity.Note) error
	FindByEmployee(ctx context.Context, employeeID string) ([]*entity.Note, error)
	FindByID(ctx context.Context, id string) (*entity.Note, error)
	Delete(ctx context.Context, id string) error
}
