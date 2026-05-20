package services

import (
	"context"
	"fmt"

	"ad-sync-manager/internal/domain/entity"
	"ad-sync-manager/internal/domain/interfaces"
)

type employeeService struct {
	ad   interfaces.ADClient
	repo interfaces.EmployeeRepository
	log  interfaces.Logger
}

// NewEmployeeService wires the employee use-case.
func NewEmployeeService(
	ad interfaces.ADClient,
	repo interfaces.EmployeeRepository,
	log interfaces.Logger,
) interfaces.EmployeeService {
	return &employeeService{ad: ad, repo: repo, log: log.With("svc", "employee")}
}

func (s *employeeService) List(ctx context.Context, filter interfaces.EmployeeFilter) ([]*entity.Employee, error) {
	employees, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("employee: list: %w", err)
	}
	return employees, nil
}

func (s *employeeService) GetByID(ctx context.Context, id string) (*entity.Employee, error) {
	emp, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("employee: get %q: %w", id, err)
	}
	return emp, nil
}
