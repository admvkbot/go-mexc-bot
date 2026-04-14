package mexcfutures

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// SubmitOrder calls POST /private/order/submit (mexc-futures-sdk).
func (c *Client) SubmitOrder(ctx context.Context, req SubmitOrderRequest) ([]byte, error) {
	return c.postSignedFutures(ctx, pathSubmitOrder, req)
}

// PlaceOrderCreate calls POST /private/order/create (Python mexc_client.place_order).
func (c *Client) PlaceOrderCreate(ctx context.Context, req PlaceOrderCreateRequest) ([]byte, error) {
	return c.postSignedFutures(ctx, pathPlaceOrderCreate, req)
}

// OpenMarketPosition calls POST /private/order/submit with the Python-shaped body.
func (c *Client) OpenMarketPosition(ctx context.Context, req OpenMarketPositionRequest) ([]byte, error) {
	return c.postSignedFutures(ctx, pathSubmitOrder, req)
}

// CancelOrder cancels up to 50 orders by numeric IDs (mexc-futures-sdk).
func (c *Client) CancelOrder(ctx context.Context, orderIDs []int64) ([]byte, error) {
	if len(orderIDs) == 0 {
		return nil, fmt.Errorf("mexcfutures: CancelOrder: orderIDs is empty")
	}
	if len(orderIDs) > 50 {
		return nil, fmt.Errorf("mexcfutures: CancelOrder: at most 50 order IDs")
	}
	return c.postSignedFutures(ctx, pathCancelOrder, orderIDs)
}

// CancelOrderByExternalID calls POST /private/order/cancel_with_external.
func (c *Client) CancelOrderByExternalID(ctx context.Context, req CancelOrderByExternalIDRequest) ([]byte, error) {
	return c.postSignedFutures(ctx, pathCancelOrderByExternalID, req)
}

// CancelAllOrders calls POST /private/order/cancel_all.
func (c *Client) CancelAllOrders(ctx context.Context, req CancelAllOrdersRequest) ([]byte, error) {
	return c.postSignedFutures(ctx, pathCancelAllOrders, req)
}

// OrderHistory calls GET /private/order/list/history_orders.
func (c *Client) OrderHistory(ctx context.Context, p OrderHistoryParams) ([]byte, error) {
	q := url.Values{}
	q.Set("category", strconv.Itoa(p.Category))
	q.Set("page_num", strconv.Itoa(p.PageNum))
	q.Set("page_size", strconv.Itoa(p.PageSize))
	q.Set("states", strconv.Itoa(p.States))
	q.Set("symbol", p.Symbol)
	return c.getAuthFutures(ctx, pathOrderHistory, q)
}

// OrderDeals calls GET /private/order/list/order_deals.
func (c *Client) OrderDeals(ctx context.Context, p OrderDealsParams) ([]byte, error) {
	q := url.Values{}
	q.Set("symbol", p.Symbol)
	if p.StartTime != 0 {
		q.Set("start_time", strconv.FormatInt(p.StartTime, 10))
	}
	if p.EndTime != 0 {
		q.Set("end_time", strconv.FormatInt(p.EndTime, 10))
	}
	q.Set("page_num", strconv.Itoa(p.PageNum))
	q.Set("page_size", strconv.Itoa(p.PageSize))
	return c.getAuthFutures(ctx, pathOrderDeals, q)
}

// GetOrder calls GET /private/order/get/{orderId}.
func (c *Client) GetOrder(ctx context.Context, orderID string) ([]byte, error) {
	path := pathGetOrder + "/" + url.PathEscape(orderID)
	return c.getAuthFutures(ctx, path, nil)
}

// GetOrderByExternalID calls GET /private/order/external/{symbol}/{externalOid}.
func (c *Client) GetOrderByExternalID(ctx context.Context, symbol, externalOid string) ([]byte, error) {
	path := pathGetOrderByExternalPrefix + "/" + url.PathEscape(symbol) + "/" + url.PathEscape(externalOid)
	return c.getAuthFutures(ctx, path, nil)
}

// DecodeJSON is a small helper for decoding API JSON bodies.
func DecodeJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
