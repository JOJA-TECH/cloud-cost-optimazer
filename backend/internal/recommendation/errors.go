package recommendation

import "errors"

var (
	ErrPolicyDisabled           = errors.New("optimization policy is disabled")
	ErrPolicyRejected           = errors.New("recommendation rejected by optimization policy")
	ErrPolicyRequired           = errors.New("optimization policy is required")
	ErrPolicyEvaluator          = errors.New("optimization policy evaluator is unavailable")
	ErrRepository               = errors.New("recommendation repository is unavailable")
	ErrInvalidRequest           = errors.New("invalid recommendation request")
	ErrConfigurationUnavailable = errors.New("RECOMMENDATION_CONFIGURATION_UNAVAILABLE")
	ErrInsufficientEvidence     = errors.New("RECOMMENDATION_INSUFFICIENT_EVIDENCE")
	ErrResourceNotFound         = errors.New("recommendation resource not found")
	ErrPolicyNotFound           = errors.New("optimization policy not found")
	ErrRecommendationNotFound   = errors.New("recommendation not found")
	ErrMLUnavailable            = errors.New("ML forecasts unavailable")
	ErrConcurrentOperation      = errors.New("incompatible recommendation operation is already in progress")
	ErrDependenciesUnavailable  = errors.New("recommendation dependencies are unavailable")
)
