package ports

import (
	"context"

	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/mexc/mexcfutures"
)

// FuturesREST is the minimal surface the application needs from a MEXC futures REST client.
type FuturesREST interface {
	TestConnection(ctx context.Context) error
	SubmitOrder(ctx context.Context, req mexcfutures.SubmitOrderRequest) ([]byte, error)
	CancelOrder(ctx context.Context, orderIDs []int64) ([]byte, error)
	CancelOrderByExternalID(ctx context.Context, req mexcfutures.CancelOrderByExternalIDRequest) ([]byte, error)
	CancelAllOrders(ctx context.Context, req mexcfutures.CancelAllOrdersRequest) ([]byte, error)
	GetOrder(ctx context.Context, orderID string) ([]byte, error)
	GetOrderByExternalID(ctx context.Context, symbol, externalOid string) ([]byte, error)
	OpenPositionsFutures(ctx context.Context, symbol *string) ([]byte, error)
}
