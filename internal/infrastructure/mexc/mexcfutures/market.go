package mexcfutures

import (
	"context"
	"net/url"
	"strconv"
)

// Ticker calls GET /contract/ticker (public).
func (c *Client) Ticker(ctx context.Context, symbol string) ([]byte, error) {
	q := url.Values{}
	q.Set("symbol", symbol)
	return c.getPublicFutures(ctx, pathTicker, q)
}

// ContractDetailFutures calls GET /contract/detail on the futures host (optional symbol).
func (c *Client) ContractDetailFutures(ctx context.Context, symbol *string) ([]byte, error) {
	var q url.Values
	if symbol != nil && *symbol != "" {
		q = url.Values{}
		q.Set("symbol", *symbol)
	}
	return c.getPublicFutures(ctx, pathContractDetail, q)
}

// ContractDepth calls GET /contract/depth/{symbol} (public).
func (c *Client) ContractDepth(ctx context.Context, symbol string, limit *int) ([]byte, error) {
	path := pathContractDepth + "/" + url.PathEscape(symbol)
	var q url.Values
	if limit != nil {
		q = url.Values{}
		q.Set("limit", strconv.Itoa(*limit))
	}
	return c.getPublicFutures(ctx, path, q)
}

// TestConnection calls Ticker for BTC_USDT (mexc-futures-sdk behavior).
func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.Ticker(ctx, "BTC_USDT")
	return err
}
