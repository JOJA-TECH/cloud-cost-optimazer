// Package valkey provides the shared Valkey client (Valkey is the Redis fork;
// valkey-go is its official Go client).
package valkey

import (
	"context"
	"os"
	"strconv"
	"time"

	valkey "github.com/valkey-io/valkey-go"
)

// NewFromEnv returns a valkey.Client configured from:
//   - CCO_VALKEY_ADDR     host:port (required to enable; empty → nil client)
//   - CCO_VALKEY_PASSWORD optional
//   - CCO_VALKEY_DB       optional int, default 0
//
// Returns (nil, nil) when CCO_VALKEY_ADDR is unset → callers fall back to
// in-memory implementations (single-instance dev mode). When set, it pings
// with a 3s timeout and returns an error if the server is unreachable.
func NewFromEnv() (valkey.Client, error) {
	addr := os.Getenv("CCO_VALKEY_ADDR")
	if addr == "" {
		return nil, nil
	}
	// Invalid/unset CCO_VALKEY_DB falls back to DB 0.
	db, _ := strconv.Atoi(os.Getenv("CCO_VALKEY_DB"))

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{addr},
		Password:    os.Getenv("CCO_VALKEY_PASSWORD"),
		SelectDB:    db,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}
