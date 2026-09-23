package domain

import (
	"context"
	"shop_project_be/internal/constant/paginated"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Customers struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name      string         `gorm:"type:varchar(150);not null" json:"name"`
	Phone     string         `gorm:"type:varchar(15)" json:"phone"`
	Address   string         `gorm:"type:text" json:"address"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (c *Customers) TableName() string {
	return "customers"
}

type FilterCustomer struct {
	Search string
	Cursor *paginated.CursorMeta
	Limit  int
	Order  string
}

type CustomersPaginated struct {
	DataItem []*Customers
	HasNext  bool
	Cursor   *paginated.CursorMeta
}

type CustomerRepository interface {
	GetCustomer(ctx context.Context, id uuid.UUID) (*Customers, error)
	UpdateCustomer(ctx context.Context, id uuid.UUID, customer *Customers) error
	AddCustomer(ctx context.Context, customer *Customers) error
	DeleteCustomer(ctx context.Context, id uuid.UUID) error
	GetAllCustomer(ctx context.Context, filter FilterCustomer) (*CustomersPaginated, error)
}

type CustomerUsecase interface {
	AddCustomerShop(ctx context.Context, request *requestdto.AddCustomer) error
	UpdateCustomerShop(ctx context.Context, request *requestdto.UpdateCustomer) error
	GetListCustomerShop(ctx context.Context, request *requestdto.GetAllCustomer) (*responsedto.ListCustomerDtoResponse, error)
	GetCustomerShop(ctx context.Context, request *requestdto.GetCustomer) (*responsedto.CustomerDtoResponse, error)
	DeleteCustomerShop(ctx context.Context, request *requestdto.DeleteCustomer) error
}
