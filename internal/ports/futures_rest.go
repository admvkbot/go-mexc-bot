package ports

import "context"

// FuturesREST is the minimal surface the application needs from a MEXC futures REST client.
type FuturesREST interface {
	TestConnection(ctx context.Context) error
}
