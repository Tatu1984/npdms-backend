package services

import (
	"context"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"

	"github.com/google/uuid"
)

type VehicleService struct {
	vehicleRepo *repository.VehicleRepository
	auditRepo   *repository.AuditRepository
}

func NewVehicleService(vehicleRepo *repository.VehicleRepository, auditRepo *repository.AuditRepository) *VehicleService {
	return &VehicleService{
		vehicleRepo: vehicleRepo,
		auditRepo:   auditRepo,
	}
}

func (s *VehicleService) List(ctx context.Context, filter repository.VehicleFilter) (*models.PaginatedResponse, error) {
	vehicles, total, err := s.vehicleRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / filter.PageSize
	if int(total)%filter.PageSize != 0 {
		totalPages++
	}

	return &models.PaginatedResponse{
		Data:       vehicles,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

// Owner answers which department the vehicle is on the strength of.
func (s *VehicleService) Owner(ctx context.Context, id, viewerID uuid.UUID) (bool, string, error) {
	return s.vehicleRepo.Owner(ctx, id, viewerID)
}

func (s *VehicleService) GetByID(ctx context.Context, id uuid.UUID) (*models.Vehicle, error) {
	return s.vehicleRepo.FindByID(ctx, id)
}

func (s *VehicleService) Create(ctx context.Context, vehicle *models.Vehicle) (*models.Vehicle, error) {
	if vehicle.Status == "" {
		vehicle.Status = models.VehicleStatusAvailable
	}

	err := s.vehicleRepo.Create(ctx, vehicle)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "vehicle_created",
		ResourceType: "vehicle",
		ResourceID:   &vehicle.ID,
		Success:      true,
	})

	return s.vehicleRepo.FindByID(ctx, vehicle.ID)
}

func (s *VehicleService) Update(ctx context.Context, vehicle *models.Vehicle) (*models.Vehicle, error) {
	err := s.vehicleRepo.Update(ctx, vehicle)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "vehicle_updated",
		ResourceType: "vehicle",
		ResourceID:   &vehicle.ID,
		Success:      true,
	})

	return s.vehicleRepo.FindByID(ctx, vehicle.ID)
}

func (s *VehicleService) AllocateVehicle(ctx context.Context, id, driverID uuid.UUID, duty string) (*models.Vehicle, error) {
	err := s.vehicleRepo.AllocateVehicle(ctx, id, driverID, duty)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "vehicle_allocated",
		ResourceType: "vehicle",
		ResourceID:   &id,
		Success:      true,
	})

	return s.vehicleRepo.FindByID(ctx, id)
}

func (s *VehicleService) ReturnVehicle(ctx context.Context, id uuid.UUID) (*models.Vehicle, error) {
	err := s.vehicleRepo.ReturnVehicle(ctx, id)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "vehicle_returned",
		ResourceType: "vehicle",
		ResourceID:   &id,
		Success:      true,
	})

	return s.vehicleRepo.FindByID(ctx, id)
}

func (s *VehicleService) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.vehicleRepo.Delete(ctx, id)
	if err != nil {
		return err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "vehicle_deleted",
		ResourceType: "vehicle",
		ResourceID:   &id,
		Success:      true,
	})

	return nil
}
