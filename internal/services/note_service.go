package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"ad-sync-manager/internal/domain/entity"
	"ad-sync-manager/internal/domain/interfaces"
)

type noteService struct {
	repo   interfaces.NoteRepository
	parser interfaces.MarkdownParser
	log    interfaces.Logger
}

// NewNoteService wires the markdown note use-case.
func NewNoteService(
	repo interfaces.NoteRepository,
	parser interfaces.MarkdownParser,
	log interfaces.Logger,
) interfaces.NoteService {
	return &noteService{repo: repo, parser: parser, log: log.With("svc", "note")}
}

func (s *noteService) Create(ctx context.Context, employeeID, authorID, md string) (*entity.Note, error) {
	src := []byte(md)
	if err := s.parser.Validate(src); err != nil {
		return nil, fmt.Errorf("note: validation: %w", err)
	}

	html, err := s.parser.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("note: render: %w", err)
	}

	now := time.Now()
	note := &entity.Note{
		ID:         uuid.NewString(),
		EmployeeID: employeeID,
		Content:    md,
		ParsedHTML: string(html),
		AuthorID:   authorID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.repo.Save(ctx, note); err != nil {
		return nil, fmt.Errorf("note: save: %w", err)
	}

	s.log.Info("note created", "id", note.ID, "employee", employeeID, "author", authorID)
	return note, nil
}

func (s *noteService) Update(ctx context.Context, noteID, authorID, md string) (*entity.Note, error) {
	note, err := s.repo.FindByID(ctx, noteID)
	if err != nil {
		return nil, fmt.Errorf("note: find %q: %w", noteID, err)
	}

	src := []byte(md)
	if err := s.parser.Validate(src); err != nil {
		return nil, fmt.Errorf("note: validation: %w", err)
	}

	html, err := s.parser.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("note: render: %w", err)
	}

	note.Content    = md
	note.ParsedHTML = string(html)
	note.UpdatedAt  = time.Now()

	if err := s.repo.Save(ctx, note); err != nil {
		return nil, fmt.Errorf("note: update save: %w", err)
	}

	return note, nil
}

func (s *noteService) ListForEmployee(ctx context.Context, employeeID string) ([]*entity.Note, error) {
	notes, err := s.repo.FindByEmployee(ctx, employeeID)
	if err != nil {
		return nil, fmt.Errorf("note: list for %q: %w", employeeID, err)
	}
	return notes, nil
}

func (s *noteService) Delete(ctx context.Context, noteID, _ string) error {
	if err := s.repo.Delete(ctx, noteID); err != nil {
		return fmt.Errorf("note: delete %q: %w", noteID, err)
	}
	s.log.Info("note deleted", "id", noteID)
	return nil
}
