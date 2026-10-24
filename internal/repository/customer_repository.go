package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type customerRepository struct {
	db *gorm.DB
}

func NewCustomerRepository(db *gorm.DB) domain.CustomerRepository {
	return &customerRepository{db: db}
}

func (c *customerRepository) AddCustomer(ctx context.Context, customer *domain.Customers) error {
	result := c.db.WithContext(ctx).Create(customer)
	if result.Error != nil {
		return fmt.Errorf("failed to add customer: %w", result.Error)
	}
	return nil
}

func (c *customerRepository) DeleteCustomer(ctx context.Context, id uuid.UUID) error {
	return runTxDB(ctx, c.db, func(tx *gorm.DB) error {
		var customer domain.Customers
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&customer).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.NotFound(fmt.Sprintf("customer with id %s not found", id))
			}
			return internalErr(fmt.Errorf("failed to lock customer: %w", err))
		}
		var openDebts int64
		if err := tx.Model(&domain.Debts{}).
			Where("customer_id = ? AND status = ?", id, enum.BELUM_LUNAS).
			Count(&openDebts).Error; err != nil {
			return internalErr(fmt.Errorf("failed to count open debts: %w", err))
		}
		if openDebts > 0 {
			return domain.Conflict(fmt.Sprintf("customer %s cannot be deleted: they still have an unpaid debt; settle it first", id))
		}
		if err := tx.Delete(&customer).Error; err != nil {
			return internalErr(fmt.Errorf("failed to delete customer: %w", err))
		}
		return nil
	})
}

func lockCustomerShared(tx *gorm.DB, id uuid.UUID) error {
	var customer domain.Customers
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Select("id").
		Where("id = ?", id).First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.NotFound(fmt.Sprintf("customer with id %s not found", id))
		}
		return internalErr(fmt.Errorf("failed to lock customer: %w", err))
	}
	return nil
}

func (c *customerRepository) GetCustomer(ctx context.Context, id uuid.UUID, includeInactive bool) (*domain.Customers, error) {
	var customer domain.Customers
	db := c.db.WithContext(ctx)
	if includeInactive {
		db = db.Unscoped()
	}
	if err := db.Where("id = ?", id).First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.NotFound(fmt.Sprintf("customer with id %s not found", id))
		}
		return nil, fmt.Errorf("failed to get customer: %w", err)
	}
	return &customer, nil
}

func (c *customerRepository) GetAllCustomer(ctx context.Context, filter domain.FilterCustomer) (*domain.CustomersPaginated, error) {
	limit, order := pageParams(filter.Limit, filter.Order)

	query := c.db.WithContext(ctx).Model(&domain.Customers{})
	if filter.Inactive {
		query = query.Unscoped().Where("deleted_at IS NOT NULL")
	}
	if filter.Search != "" {
		query = query.Where("name ILIKE ? ESCAPE '\\'", likeContains(filter.Search))
	}

	var items []*domain.Customers
	if err := keysetPage(query, "", limit, order, filter.Cursor).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("failed to get customers: %w", err)
	}
	items, hasNext, next := trimPage(items, limit, func(c *domain.Customers) (time.Time, uuid.UUID) { return c.CreatedAt, c.ID })

	return &domain.CustomersPaginated{
		DataItem: items,
		HasNext:  hasNext,
		Cursor:   next,
	}, nil
}

func (c *customerRepository) UpdateCustomer(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	result := c.db.WithContext(ctx).Model(&domain.Customers{}).Where("id = ?", id).Updates(fields)
	if result.Error != nil {
		return fmt.Errorf("failed to update customer: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NotFound(fmt.Sprintf("customer with id %s not found", id))
	}
	return nil
}

func (c *customerRepository) RestoreCustomer(ctx context.Context, id uuid.UUID) error {
	result := c.db.WithContext(ctx).Unscoped().Model(&domain.Customers{}).
		Where("id = ? AND deleted_at IS NOT NULL", id).Update("deleted_at", nil)
	if result.Error != nil {
		return fmt.Errorf("failed to restore customer: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NotFound(fmt.Sprintf("inactive customer with id %s not found", id))
	}
	return nil
}

func (c *customerRepository) PurgeCustomer(ctx context.Context, id uuid.UUID, anonFields map[string]any) (bool, error) {
	var anonymized bool
	err := runTxDB(ctx, c.db, func(tx *gorm.DB) error {
		var customer domain.Customers
		if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&customer).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.NotFound(fmt.Sprintf("customer with id %s not found", id))
			}
			return internalErr(fmt.Errorf("failed to lock customer: %w", err))
		}
		if !customer.DeletedAt.Valid {
			return domain.Conflict("deactivate the customer before deleting it permanently")
		}
		var openDebts int64
		if err := tx.Model(&domain.Debts{}).
			Where("customer_id = ? AND status = ?", id, enum.BELUM_LUNAS).
			Count(&openDebts).Error; err != nil {
			return internalErr(fmt.Errorf("failed to count open debts: %w", err))
		}
		if openDebts > 0 {
			return domain.Conflict("customer still has an unpaid debt; settle it first")
		}

		var referenced bool
		if err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM transactions WHERE customer_id = @id)
			OR EXISTS (SELECT 1 FROM debts WHERE customer_id = @id)
			OR EXISTS (SELECT 1 FROM payments WHERE customer_id = @id)`,
			sql.Named("id", id)).Scan(&referenced).Error; err != nil {
			return internalErr(fmt.Errorf("failed to check customer history: %w", err))
		}
		anonymized = referenced
		if referenced {
			if err := tx.Unscoped().Model(&domain.Customers{}).Where("id = ?", id).Updates(anonFields).Error; err != nil {
				return internalErr(fmt.Errorf("failed to anonymize customer: %w", err))
			}
			return nil
		}
		if err := tx.Unscoped().Delete(&domain.Customers{}, "id = ?", id).Error; err != nil {
			return internalErr(fmt.Errorf("failed to delete customer: %w", err))
		}
		return nil
	})
	return anonymized, err
}
