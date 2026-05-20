package interfaces

import (
	"context"

	"ad-sync-manager/internal/domain/entity"
)

// EmployeeService is the port for the employee use-case.
type EmployeeService interface {
	List(ctx context.Context, filter EmployeeFilter) ([]*entity.Employee, error)
	GetByID(ctx context.Context, id string) (*entity.Employee, error)
}
