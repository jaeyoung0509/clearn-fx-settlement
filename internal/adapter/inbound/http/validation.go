package httpadapter

import (
	"errors"

	"github.com/go-playground/validator/v10"

	"fx-settlement-lab/go-backend/internal/domain"
)

func requestValidationError(err error) *domain.AppError {
	details := map[string]any{
		"errors": []map[string]string{
			{"message": err.Error()},
		},
	}

	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		issues := make([]map[string]string, 0, len(validationErrors))
		for _, validationError := range validationErrors {
			issues = append(issues, map[string]string{
				"field": validationError.Field(),
				"tag":   validationError.Tag(),
			})
		}
		details["errors"] = issues
	}

	return domain.Validation("Request validation failed", details)
}
