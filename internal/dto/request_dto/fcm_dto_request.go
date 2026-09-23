package requestdto

type RegisterDeviceRequest struct {
	Token    string `json:"token" validate:"required"`
	Platform string `json:"platform" validate:"required,oneof=android ios"`
	DeviceID string `json:"device_id" validate:"required"`
}

type LogoutDeviceRequest struct {
	Token string `json:"token" validate:"required"`
}
