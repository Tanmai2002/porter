package gorm

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/porter-dev/porter/internal/models"
	"github.com/porter-dev/porter/internal/repository"
	"gorm.io/gorm"
)

// PorterAppEventRepository uses gorm.DB for querying the database
type PorterAppEventRepository struct {
	db *gorm.DB
}

// NewPorterAppEventRepository returns a PorterAppEventRepository which uses
// gorm.DB for querying the database
func NewPorterAppEventRepository(db *gorm.DB) repository.PorterAppEventRepository {
	return &PorterAppEventRepository{db}
}

func (repo *PorterAppEventRepository) ListEventsByPorterAppID(porterAppID uint) ([]*models.PorterAppEvent, error) {
	apps := []*models.PorterAppEvent{}

	id := strconv.Itoa(int(porterAppID))
	if id == "" {
		return nil, errors.New("invalid porter app id supplied")
	}

	if err := repo.db.Where("porter_app_id = ?", id).Find(&apps).Error; err != nil {
		fmt.Println("STEFAN", err)
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	return apps, nil
}

func (repo *PorterAppEventRepository) EventByID(eventID uuid.UUID) (*models.PorterAppEvent, error) {
	app := &models.PorterAppEvent{}

	if eventID == uuid.Nil {
		return app, errors.New("invalid porter app event id supplied")
	}

	tx := repo.db.Find(&app, "id = ?", eventID.String())
	if tx.Error != nil {
		return app, fmt.Errorf("no porter app event found for id %s: %w", eventID, tx.Error)
	}

	return app, nil
}