package recommendation

import (
	"fmt"
	"strings"
)

func EvaluatePolicy(policy OptimizationPolicy, input PolicyEvaluationInput) PolicyEvaluation {
	result := PolicyEvaluation{PolicyUID: policy.UID, PolicyVersion: policy.Version}
	if !policy.Enabled {
		result.ViolatedConstraints = append(result.ViolatedConstraints, "policy_disabled")
		return result
	}
	if input.Action.Type == ActionNoAction {
		result.Allowed = true
		result.Warnings = []string{"no_action_is_auditable_and_does_not_apply_infrastructure_changes"}
		return result
	}

	constraints := policy.Constraints
	if maximum, ok := numeric(constraints["max_risk_score"]); ok {
		if input.Risk.Score > maximum {
			result.ViolatedConstraints = append(result.ViolatedConstraints, "max_risk_score")
		}
	}
	if minimum, ok := numeric(constraints["min_confidence"]); ok && input.Confidence.Score < minimum {
		result.ViolatedConstraints = append(result.ViolatedConstraints, "min_confidence")
	}
	if boolConstraint(constraints, "allow_scale_down") == false &&
		input.Action.Type == ActionResizeDown {
		result.ViolatedConstraints = append(result.ViolatedConstraints, "allow_scale_down")
	}
	if boolConstraint(constraints, "allow_compute_model_change") == false &&
		input.Action.Type == ActionChangeComputeModel {
		result.ViolatedConstraints = append(result.ViolatedConstraints, "allow_compute_model_change")
	}
	if input.Simulation.Feasible == nil || !*input.Simulation.Feasible {
		result.ViolatedConstraints = append(result.ViolatedConstraints, "simulation_infeasible")
	}
	if maximum, ok := numeric(constraints["max_sla_violations"]); ok {
		if input.Simulation.PredictedSLAViolations == nil {
			result.ViolatedConstraints = append(result.ViolatedConstraints, "sla_evidence_unavailable")
		} else if float64(*input.Simulation.PredictedSLAViolations) > maximum {
			result.ViolatedConstraints = append(result.ViolatedConstraints, "max_sla_violations")
		}
	}
	if _, ok := numeric(constraints["max_latency_increase_pct"]); ok && input.Simulation.PredictedLatencyP95MS == nil {
		result.ViolatedConstraints = append(result.ViolatedConstraints, "latency_evidence_unavailable")
	}
	if len(result.ViolatedConstraints) == 0 {
		result.Allowed = true
	} else {
		result.Warnings = append(result.Warnings, fmt.Sprintf("candidate rejected: %s", strings.Join(result.ViolatedConstraints, ",")))
	}
	return result
}

func boolConstraint(constraints map[string]any, key string) bool {
	value, exists := constraints[key]
	if !exists {
		return false
	}
	allowed, ok := value.(bool)
	return ok && allowed
}
