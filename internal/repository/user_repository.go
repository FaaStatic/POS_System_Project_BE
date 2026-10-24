package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"shop_project_be/internal/domain"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

func (u *userRepository) GetUserAnyState(ctx context.Context, id uuid.UUID) (*domain.Users, error) {
	var userData domain.Users
	if err := u.db.WithContext(ctx).Unscoped().Where("id = ?", id).First(&userData).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.NotFound("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &userData, nil
}

func (u *userRepository) ListUsers(ctx context.Context, filter domain.FilterUser) (*domain.UsersPaginated, error) {
	limit, order := pageParams(filter.Limit, filter.Order)

	query := u.db.WithContext(ctx).Model(&domain.Users{})
	if filter.Inactive {
		query = query.Unscoped().Where("deleted_at IS NOT NULL")
	}
	if filter.Search != "" {
		query = query.Where("username ILIKE ? ESCAPE '\\'", likeContains(filter.Search))
	}

	var items []*domain.Users
	if err := keysetPage(query, "", limit, order, filter.Cursor).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	items, hasNext, next := trimPage(items, limit, func(x *domain.Users) (time.Time, uuid.UUID) { return x.CreatedAt, x.ID })
	return &domain.UsersPaginated{DataItem: items, HasNext: hasNext, Cursor: next}, nil
}

func (u *userRepository) UpdateUser(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	result := u.db.WithContext(ctx).Model(&domain.Users{}).Where("id = ?", id).Updates(fields)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return domain.Duplicate("username already taken")
		}
		return fmt.Errorf("failed to update user: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NotFound("user not found")
	}
	return nil
}

func (u *userRepository) DeactivateUser(ctx context.Context, id uuid.UUID) error {
	result := u.db.WithContext(ctx).Where("id = ?", id).Delete(&domain.Users{})
	if result.Error != nil {
		return fmt.Errorf("failed to deactivate user: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NotFound("active user not found")
	}
	return nil
}

func (u *userRepository) RestoreUser(ctx context.Context, id uuid.UUID) error {
	result := u.db.WithContext(ctx).Unscoped().Model(&domain.Users{}).
		Where("id = ? AND deleted_at IS NOT NULL", id).Update("deleted_at", nil)
	if result.Error != nil {
		return fmt.Errorf("failed to restore user: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NotFound("inactive user not found")
	}
	return nil
}

func (u *userRepository) PurgeUser(ctx context.Context, id uuid.UUID, anonFields map[string]any) (bool, error) {
	var anonymized bool
	err := runTxDB(ctx, u.db, func(tx *gorm.DB) error {
		var user domain.Users
		if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.NotFound("user not found")
			}
			return internalErr(fmt.Errorf("failed to lock user: %w", err))
		}
		if !user.DeletedAt.Valid {
			return domain.Conflict("deactivate the user before deleting it permanently")
		}

		var referenced bool
		if err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM transactions WHERE user_id = @id)
			OR EXISTS (SELECT 1 FROM payments WHERE user_id = @id)
			OR EXISTS (SELECT 1 FROM debt_payments WHERE user_id = @id)
			OR EXISTS (SELECT 1 FROM debt_adjustments WHERE user_id = @id)`,
			sql.Named("id", id)).Scan(&referenced).Error; err != nil {
			return internalErr(fmt.Errorf("failed to check user history: %w", err))
		}

		if err := tx.Where("user_id = ?", id).Delete(&domain.DeviceToken{}).Error; err != nil {
			return internalErr(fmt.Errorf("failed to delete device tokens: %w", err))
		}
		anonymized = referenced
		if referenced {
			if err := tx.Unscoped().Model(&domain.Users{}).Where("id = ?", id).Updates(anonFields).Error; err != nil {
				return internalErr(fmt.Errorf("failed to anonymize user: %w", err))
			}
			return nil
		}
		if err := tx.Unscoped().Delete(&domain.Users{}, "id = ?", id).Error; err != nil {
			return internalErr(fmt.Errorf("failed to delete user: %w", err))
		}
		return nil
	})
	return anonymized, err
}
