package validator

import "github.com/go-playground/validator/v10"

type StructValidator struct {
	validate *validator.Validate
}

func New() *StructValidator {
	return &StructValidator{
		validate: validator.New(validator.WithRequiredStructEnabled()),
	}
}

func (v *StructValidator) Validate(out any) error {
	return v.validate.Struct(out)
}
