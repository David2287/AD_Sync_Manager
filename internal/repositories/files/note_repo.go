package files

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"ad-sync-manager/internal/domain/entity"
	"ad-sync-manager/internal/domain/interfaces"
)

// noteRepo stores notes as JSON files under baseDir/<employeeID>/<noteID>.json
// This is intentionally simple for Stage 1. Stage 2 can migrate to SQLite.
type noteRepo struct {
	baseDir string
}

// NewNoteRepo returns an interfaces.NoteRepository backed by the filesystem.
// The baseDir is created on first use.
func NewNoteRepo(baseDir string) interfaces.NoteRepository {
	return &noteRepo{baseDir: baseDir}
}

func (r *noteRepo) Save(_ context.Context, note *entity.Note) error {
	dir := filepath.Join(r.baseDir, note.EmployeeID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("notes: mkdir %q: %w", dir, err)
	}

	path := filepath.Join(dir, note.ID+".json")
	data, err := json.MarshalIndent(note, "", "  ")
	if err != nil {
		return fmt.Errorf("notes: marshal: %w", err)
	}

	if err := os.WriteFile(path, data, 0o640); err != nil {
		return fmt.Errorf("notes: write %q: %w", path, err)
	}
	return nil
}

func (r *noteRepo) FindByEmployee(_ context.Context, employeeID string) ([]*entity.Note, error) {
	dir := filepath.Join(r.baseDir, employeeID)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("notes: readdir %q: %w", dir, err)
	}

	var notes []*entity.Note
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		note, err := r.readNote(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	return notes, nil
}

func (r *noteRepo) FindByID(_ context.Context, id string) (*entity.Note, error) {
	// Walk employee dirs to find the note by ID.
	// Acceptable for Stage 1 given low note volume; index in Stage 2.
	var found *entity.Note
	err := filepath.WalkDir(r.baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		if filepath.Base(path) == id+".json" {
			note, err := r.readNote(path)
			if err != nil {
				return err
			}
			found = note
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("notes: find %q: %w", id, err)
	}
	if found == nil {
		return nil, fmt.Errorf("note %q not found", id)
	}
	return found, nil
}

func (r *noteRepo) Delete(_ context.Context, id string) error {
	var target string
	_ = filepath.WalkDir(r.baseDir, func(path string, d os.DirEntry, _ error) error {
		if !d.IsDir() && filepath.Base(path) == id+".json" {
			target = path
		}
		return nil
	})
	if target == "" {
		return fmt.Errorf("note %q not found", id)
	}
	return os.Remove(target)
}

func (r *noteRepo) readNote(path string) (*entity.Note, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("notes: read %q: %w", path, err)
	}
	var note entity.Note
	if err := json.Unmarshal(data, &note); err != nil {
		return nil, fmt.Errorf("notes: unmarshal %q: %w", path, err)
	}
	return &note, nil
}
