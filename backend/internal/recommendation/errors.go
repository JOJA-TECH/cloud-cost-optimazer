package recommendation

import "errors"

var (
	ErrPolicyDisabled  = errors.New("optimization policy is disabled")
	ErrPolicyRejected  = errors.New("recommendation rejected by optimization policy")
	ErrPolicyRequired  = errors.New("optimization policy is required")
	ErrPolicyEvaluator = errors.New("optimization policy evaluator is unavailable")
	ErrRepository      = errors.New("recommendation repository is unavailable")
)
