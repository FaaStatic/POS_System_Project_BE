package repository

import (
	"context"
	"errors"
	"fmt"
	"shop_project_be/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) domain.UserRepository {
	return &userRepository{db: db}
}

func (u *userRepository) GetUserById(ctx context.Context, id uuid.UUID) (*domain.Users, error) {
	var userData domain.Users
	result := u.db.WithContext(ctx).Where("id = ?", id).First(&userData)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.NotFound("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", result.Error)
	}
	return &userData, nil
}

func (u *userRepository) GetUserByUsername(ctx context.Context, username string) (*domain.Users, error) {
	var userData domain.Users
	result := u.db.WithContext(ctx).Where("username = ?", username).First(&userData)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by username: %w", result.Error)
	}

	return &userData, nil
}

func (u *userRepository) RegisterUser(ctx context.Context, user *domain.Users) error {
	if err := u.db.WithContext(ctx).Create(user).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return domain.Duplicate("user already exists")
		}
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}
