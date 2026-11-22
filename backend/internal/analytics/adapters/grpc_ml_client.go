package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/analytics"
	forecastv1 "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/forecast/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/uuid"
)

// GrpcMLClient implements analytics.MLPort over gRPC against the external
// Python ML service (Module 6). It translates the Go forecast contract into
// the server's PredictRequest and returns the forecast_uid that the Python
// service persists in ml.forecast. The returned forecast_uid is the shared
// public reference that Go can re-query via contract.v_forecast.
//
// The Python service owns ml.forecast persistence (Slice 2). The adapter only
// bridges transport and translates errors; it never generates a local UID
// when the server did not persist a forecast.
type GrpcMLClient struct {
	conn   *grpc.ClientConn
	client forecastv1.ForecastServiceClient
	timeout time.Duration
}

// NewGRPCMLClient connects to the Python ML service endpoint (loopback by
// default, suitable for co-located dev). Non-local endpoints should use TLS,
// which is not yet configured in this slice.
func NewGRPCMLClient(endpoint string, timeout time.Duration) (*GrpcMLClient, error) {
	if endpoint == "" {
		endpoint = "127.0.0.1:50051"
	}
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	conn, err := grpc.NewClient(endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect ML endpoint %s: %w", endpoint, err)
	}
	return &GrpcMLClient{
		conn:    conn,
		client:  forecastv1.NewForecastServiceClient(conn),
		timeout: timeout,
	}, nil
}

// Close releases the gRPC connection.
func (c *GrpcMLClient) Close() error {
	return c.conn.Close()
}

// ValidateContract confirms the ML service is reachable and serving a model.
func (c *GrpcMLClient) ValidateContract(ctx context.Context) error {
	if _, err := c.client.GetModelInfo(ctx, &forecastv1.GetModelInfoRequest{}); err != nil {
		return analytics.NewError(analytics.CodeMLUnavailable, "ML contract not ready: "+err.Error(), analytics.ErrMLUnavailable)
	}
	return nil
}

// Forecast asks the Python ML service for a prediction and returns a
// forecast_uid. The server validates target/horizon/model itself; this adapter
// maps canonical Go names to the server's target names and translates gRPC
// status codes into typed analytics errors.
func (c *GrpcMLClient) Forecast(ctx context.Context, req analytics.ForecastRequest) (uuid.UUID, error) {
	if req.ResourceUID == uuid.Nil {
		return uuid.Nil, analytics.NewError(analytics.CodeInvalidQuery, "resource_uid required", analytics.ErrInvalidQuery)
	}
	if req.TargetMetric == "" {
		return uuid.Nil, analytics.NewError(analytics.CodeInvalidQuery, "target_metric required", analytics.ErrInvalidQuery)
	}
	if req.HorizonSteps <= 0 {
		return uuid.Nil, analytics.NewError(analytics.CodeInvalidQuery, "horizon_steps required", analytics.ErrInvalidQuery)
	}

	target, err := targetToServer(req.TargetMetric)
	if err != nil {
		return uuid.Nil, analytics.NewError(analytics.CodeInvalidQuery, err.Error(), analytics.ErrInvalidQuery)
	}
	horizon, err := horizonToServer(req.HorizonSteps, req.StepUnit)
	if err != nil {
		return uuid.Nil, analytics.NewError(analytics.CodeInvalidQuery, err.Error(), analytics.ErrInvalidQuery)
	}

	if req.ModelVersionID != nil {
		// The Python server resolves model versions by semver string, not by the
		// ml.model_version surrogate id. An explicit id cannot be resolved yet.
		return uuid.Nil, analytics.NewError(analytics.CodeInvalidQuery,
			"explicit model_version_id not supported by the ML server; request champion (nil)",
			analytics.ErrInvalidQuery,
		)
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	serverReq := &forecastv1.PredictRequest{
		ResourceUid:    req.ResourceUID.String(),
		Target:         target,
		HorizonMinutes: horizon,
		RequestId:      req.RequestID,
	}
	if !req.PeriodStartUTC.IsZero() {
		serverReq.CutoffTime = timestamppb.New(req.PeriodStartUTC)
	}

	resp, err := c.client.Predict(ctx, serverReq)
	if err != nil {
		slog.Error("ML Predict failed", "resource_uid", req.ResourceUID, "target", req.TargetMetric, "error", err)
		return uuid.Nil, c.mapGrpcError(err)
	}

	if resp.Status != "ok" {
		return uuid.Nil, analytics.NewError(analytics.CodeMLUnavailable,
			fmt.Sprintf("ML service returned status %q for resource %s", resp.Status, req.ResourceUID),
			analytics.ErrMLUnavailable,
		)
	}

	if resp.ForecastUid == "" {
		return uuid.Nil, analytics.NewError(analytics.CodeMLUnavailable,
			fmt.Sprintf("ML service did not persist a forecast for resource %s", req.ResourceUID),
			analytics.ErrMLUnavailable,
		)
	}

	forecastUID, err := uuid.Parse(resp.ForecastUid)
	if err != nil {
		return uuid.Nil, analytics.NewError(analytics.CodeMLUnavailable,
			fmt.Sprintf("ML service returned invalid forecast_uid %q: %v", resp.ForecastUid, err),
			analytics.ErrMLUnavailable,
		)
	}

	slog.Info("ML forecast produced", "forecast_uid", forecastUID, "resource_uid", req.ResourceUID, "target", req.TargetMetric, "model", resp.ModelVersion)
	return forecastUID, nil
}

// Train requests the ML service to train a new candidate model for a
// resource/target and register it in ml.training_run / ml.model_version.
// This is a management operation, not part of MLPort (inference-only).
func (c *GrpcMLClient) Train(
	ctx context.Context,
	resourceUID uuid.UUID,
	target, modelName, modelVersion, algorithm string,
	hours int32,
) (*forecastv1.TrainResponse, error) {
	targetServer, err := targetToServer(target)
	if err != nil {
		return nil, analytics.NewError(analytics.CodeInvalidQuery, err.Error(), analytics.ErrInvalidQuery)
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.Train(ctx, &forecastv1.TrainRequest{
		ResourceUid: resourceUID.String(),
		Target:      targetServer,
		ModelName:   modelName,
		ModelVersion: modelVersion,
		Algorithm:   algorithm,
		Hours:       hours,
	})
	if err != nil {
		return nil, c.mapGrpcError(err)
	}
	return resp, nil
}

// targetToServer maps the canonical target names used by the Go contract to the
// target names served by the Python model artifact.
func targetToServer(target string) (string, error) {
	switch target {
	case "cpu_avg":
		return "target_cpu_avg_next1h", nil
	case "workload_intensity":
		return "target_workload_next1h", nil
	case "storage_used_bytes":
		return "target_storage_next1h", nil
	default:
		return "", fmt.Errorf("unsupported target_metric %q", target)
	}
}

// horizonToServer validates the Go horizon and returns minutes for the server.
// Release 1.0 supports only a single 1-hour step.
func horizonToServer(steps int, unit string) (int32, error) {
	if unit != "1h" || steps != 1 {
		return 0, fmt.Errorf("unsupported horizon %d %s; only 1x 1h is supported", steps, unit)
	}
	return 60, nil
}

func (c *GrpcMLClient) mapGrpcError(err error) error {
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.InvalidArgument:
			return analytics.NewError(analytics.CodeInvalidQuery, st.Message(), analytics.ErrInvalidQuery)
		case codes.NotFound:
			return analytics.NewError(analytics.CodeNotFound, st.Message(), analytics.ErrNotFound)
		case codes.FailedPrecondition:
			return analytics.NewError(analytics.CodeMLUnavailable, st.Message(), analytics.ErrMLUnavailable)
		case codes.Unavailable:
			return analytics.NewError(analytics.CodeMLUnavailable, st.Message(), analytics.ErrMLUnavailable)
		case codes.DeadlineExceeded:
			return analytics.NewError(analytics.CodeMLUnavailable, "ML request timeout", err)
		default:
			return analytics.NewError(analytics.CodeMLUnavailable, st.Message(), err)
		}
	}
	return analytics.NewError(analytics.CodeMLUnavailable, err.Error(), err)
}
