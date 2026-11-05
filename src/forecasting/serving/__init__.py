"""Online inference service for the forecasting module (gRPC).

This package is independent of the training CLI. It loads the model artifact
once at startup and exposes a versioned gRPC contract for the Go backend.
"""
