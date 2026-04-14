package mexcfutures

import (
	"context"
	"net/url"
	"strconv"
)

// RiskLimit calls GET /private/account/risk_limit.
func (c *Client) RiskLimit(ctx context.Context) ([]byte, error) {
	return c.getAuthFutures(ctx, pathRiskLimit, nil)
}

// FeeRate calls GET /private/account/contract/fee_rate.
func (c *Client) FeeRate(ctx context.Context) ([]byte, error) {
	return c.getAuthFutures(ctx, pathFeeRate, nil)
}

// AccountAsset calls GET /private/account/asset/{currency}.
func (c *Client) AccountAsset(ctx context.Context, currency string) ([]byte, error) {
	path := pathAccountAsset + "/" + url.PathEscape(currency)
	return c.getAuthFutures(ctx, path, nil)
}

// OpenPositionsFutures calls GET /private/position/open_positions on the futures host (TS SDK).
func (c *Client) OpenPositionsFutures(ctx context.Context, symbol *string) ([]byte, error) {
	var q url.Values
	if symbol != nil && *symbol != "" {
		q = url.Values{}
		q.Set("symbol", *symbol)
	}
	return c.getAuthFutures(ctx, pathOpenPositionsFutures, q)
}

// PositionHistory calls GET /private/position/list/history_positions.
func (c *Client) PositionHistory(ctx context.Context, p PositionHistoryParams) ([]byte, error) {
	q := url.Values{}
	if p.Symbol != "" {
		q.Set("symbol", p.Symbol)
	}
	if p.Type != 0 {
		q.Set("type", strconv.Itoa(p.Type))
	}
	q.Set("page_num", strconv.Itoa(p.PageNum))
	q.Set("page_size", strconv.Itoa(p.PageSize))
	return c.getAuthFutures(ctx, pathPositionHistory, q)
}
