package requestdto

type UserLoginRequest struct {
	// AdminOnly diisi handler untuk login dari CMS web: staff ditolak.
	AdminOnly bool   `json:"-"`
	Username  string `json:"username" validate:"required,min=3,max=100"`
	Password  string `json:"password" validate:"required,min=6"`
}

type UserRegisterRequest struct {
	Username string `json:"username" validate:"required,min=3,max=100"`
	Password string `json:"password" validate:"required,min=6"`
}

type UserRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type UserLogoutRequest struct {
	RefreshToken string `json:"refresh_token,omitempty"`
	FcmToken     string `json:"fcm_token,omitempty"`
}

type ListStaffRequest struct {
	Status    string  `query:"status" validate:"omitempty,oneof=active inactive"`
	Search    string  `query:"search"`
	Limit     int     `query:"limit" validate:"omitempty,min=1,max=100"`
	Order     string  `query:"order" validate:"omitempty,oneof=asc desc ASC DESC"`
	AfterID   *string `query:"after_id,omitempty"`
	AfterTime *string `query:"after_time,omitempty"`
}

// UpdateStaffRequest: field nil = tidak diubah.
type UpdateStaffRequest struct {
	ID       string  `json:"-" validate:"required,uuid"`
	Username *string `json:"username,omitempty" validate:"omitempty,min=3,max=100"`
	Password *string `json:"password,omitempty" validate:"omitempty,min=6"`
}

// PurgeStaffRequest: Confirm harus sama dengan username, pengaman dari salah klik.
type PurgeStaffRequest struct {
	ID      string `json:"-" validate:"required,uuid"`
	Confirm string `json:"confirm" validate:"required"`
}
