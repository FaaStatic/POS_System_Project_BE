package usecase

import (
	"context"
	"errors"
	"fmt"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func toCustomerResponse(c *domain.Customers) responsedto.CustomerDtoResponse {
	return responsedto.CustomerDtoResponse{
		ID:     c.ID,
		Nama:   c.Name,
		NoHP:   c.Phone,
		Alamat: c.Address,
	}
}

type customerUsecase struct {
	customerRepo domain.CustomerRepository
	log          *zap.Logger
}

func NewCustomerUsecase(customerRepo domain.CustomerRepository, log *zap.Logger) domain.CustomerUsecase {
	return &customerUsecase{
		customerRepo: customerRepo,
		log:          log,
	}
}

func (c *customerUsecase) AddCustomerShop(ctx context.Context, request *requestdto.AddCustomer) error {
	customer := &domain.Customers{
		Name:    request.CustomerName,
		Phone:   request.PhoneNumber,
		Address: request.Address,
	}
	if err := c.customerRepo.AddCustomer(ctx, customer); err != nil {
		c.log.Error("failed to add customer", zap.Error(err))
		return fmt.Errorf("failed to add customer: %w", domain.ErrInternal)
	}
	return nil
}

func (c *customerUsecase) DeleteCustomerShop(ctx context.Context, request *requestdto.DeleteCustomer) error {
	id, err := uuid.Parse(request.CustomerId)
	if err != nil {
		c.log.Error("failed to parse customer id", zap.Error(err))
		return domain.InvalidID("invalid customer id format")
	}
	if err := c.customerRepo.DeleteCustomer(ctx, id); err != nil {
		c.log.Error("failed to delete customer", zap.Error(err))
		if errors.Is(err, domain.ErrInternal) {
			return fmt.Errorf("failed to delete customer: %w", domain.ErrInternal)
		}
		return err
	}
	return nil
}

func (c *customerUsecase) GetCustomerShop(ctx context.Context, request *requestdto.GetCustomer) (*responsedto.CustomerDtoResponse, error) {
	id, err := uuid.Parse(request.CustomerId)
	if err != nil {
		c.log.Error("failed to parse customer id", zap.Error(err))
		return nil, domain.InvalidID("invalid customer id format")
	}

	customer, err := c.customerRepo.GetCustomer(ctx, id)
	if err != nil {
		c.log.Error("failed to get customer", zap.Error(err))
		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get customer: %w", domain.ErrInternal)
	}

	response := toCustomerResponse(customer)
	return &response, nil
}

func (c *customerUsecase) GetListCustomerShop(ctx context.Context, request *requestdto.GetAllCustomer) (*responsedto.ListCustomerDtoResponse, error) {
	cursor, err := parseCursor(request.AfterID, request.AfterTime)
	if err != nil {
		return nil, err
	}

	filter := domain.FilterCustomer{
		Search: request.Search,
		Cursor: cursor,
		Limit:  request.Limit,
		Order:  request.Order,
	}

	result, err := c.customerRepo.GetAllCustomer(ctx, filter)
	if err != nil {
		c.log.Error("failed to get customers", zap.Error(err))
		return nil, fmt.Errorf("failed to get customers: %w", domain.ErrInternal)
	}

	responses := make([]responsedto.CustomerDtoResponse, 0, len(result.DataItem))
	for _, item := range result.DataItem {
		responses = append(responses, toCustomerResponse(item))
	}

	nextId, nextTime := result.Cursor.Encode()

	return &responsedto.ListCustomerDtoResponse{
		AfterId:       nextId,
		AfterTime:     nextTime,
		HasNext:       result.HasNext,
		CustomerLists: responses,
	}, nil
}

func (c *customerUsecase) UpdateCustomerShop(ctx context.Context, request *requestdto.UpdateCustomer) error {
	id, err := uuid.Parse(request.CustomerId)
	if err != nil {
		c.log.Error("failed to parse customer id", zap.Error(err))
		return domain.InvalidID("invalid customer id format")
	}

	customer := &domain.Customers{
		Name:    request.CustomerName,
		Phone:   request.PhoneNumber,
		Address: request.Address,
	}
	if err := c.customerRepo.UpdateCustomer(ctx, id, customer); err != nil {
		c.log.Error("failed to update customer", zap.Error(err))
		if errors.Is(err, domain.ErrNotFound) {
			return err
		}
		return fmt.Errorf("failed to update customer: %w", domain.ErrInternal)
	}
	return nil
}
