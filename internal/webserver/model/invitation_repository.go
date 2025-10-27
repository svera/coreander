package model

import (
	"errors"
	"log"

	"gorm.io/gorm"
)

type InvitationRepository struct {
	DB *gorm.DB
}

func (i *InvitationRepository) Create(invitation *Invitation) error {
	if result := i.DB.Create(invitation); result.Error != nil {
		log.Printf("error creating invitation: %s\n", result.Error)
		return result.Error
	}
	return nil
}

func (i *InvitationRepository) FindByUUID(uuid string) (*Invitation, error) {
	var invitation Invitation

	result := i.DB.Where("uuid = ?", uuid).First(&invitation)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &invitation, result.Error
}

func (i *InvitationRepository) DeleteByEmail(email string) error {
	var invitation Invitation

	result := i.DB.Where("email = ?", email).Delete(&invitation)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		log.Printf("error deleting invitation: %s\n", result.Error)
	}
	return nil
}
