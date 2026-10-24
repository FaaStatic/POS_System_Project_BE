package domain

import (
	"context"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/constant/paginated"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Users struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Username     string         `gorm:"type:varchar(100)" json:"username"`
	Password     string         `gorm:"type:varchar(255)" json:"-"`
	Role         enum.UserRole  `gorm:"type:smallint" json:"role"`
	Transactions []Transactions `gorm:"foreignKey:UserID" json:"transactions,omitempty"`
	CreatedAt    time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-"`
}

func (user *Users) HashPswd() error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.Password = string(hashed)
	return nil
}

func (user *Users) ComparedPwd(pwdUser string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(pwdUser))
	return err == nil
}

func (u *Users) TableName() string {
	return "users"
}

type FilterUser struct {
	Search   string
	Inactive bool
	Cursor   *paginated.CursorMeta
	Limit    int
	Order    string
}

type UsersPaginated struct {
	DataItem []*Users
	HasNext  bool
	Cursor   *paginated.CursorMeta
}

type UserRepository interface {
	RegisterUser(ctx context.Context, user *Users) error
	GetUserByUsername(ctx context.Context, username string) (*Users, error)
	GetUserById(ctx context.Context, id uuid.UUID) (*Users, error)
	// GetUserAnyState ikut membaca user yang dinonaktifkan.
	GetUserAnyState(ctx context.Context, id uuid.UUID) (*Users, error)
	ListUsers(ctx context.Context, filter FilterUser) (*UsersPaginated, error)
	UpdateUser(ctx context.Context, id uuid.UUID, fields map[string]any) error
	DeactivateUser(ctx context.Context, id uuid.UUID) error
	RestoreUser(ctx context.Context, id uuid.UUID) error
	// PurgeUser menghapus user nonaktif secara permanen. User yang masih dirujuk
	// riwayat (transaksi, pembayaran, hutang) tidak bisa dihapus tanpa merusak
	// laporan, jadi barisnya diisi anonFields dan tetap ada.
	PurgeUser(ctx context.Context, id uuid.UUID, anonFields map[string]any) (anonymized bool, err error)
}

type UserUsecase interface {
	UserLogin(ctx context.Context, userDto *requestdto.UserLoginRequest) (*responsedto.UserLoginResponse, error)
	RegisterUser(ctx context.Context, userDto *requestdto.UserRegisterRequest) (*responsedto.UserRegisterResponse, error)
	RefreshToken(ctx context.Context, refreshDto *requestdto.UserRefreshTokenRequest) (*responsedto.UserLoginResponse, error)
	Logout(ctx context.Context, accessToken string, logoutDto *requestdto.UserLogoutRequest) error

	ListStaff(ctx context.Context, request *requestdto.ListStaffRequest) (*responsedto.ListStaffResponse, error)
	UpdateStaff(ctx context.Context, request *requestdto.UpdateStaffRequest) error
	DeactivateStaff(ctx context.Context, id string) error
	RestoreStaff(ctx context.Context, id string) error
	PurgeStaff(ctx context.Context, request *requestdto.PurgeStaffRequest) (*responsedto.PurgeResponse, error)
}
