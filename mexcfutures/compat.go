package mexcfutures

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// GetOpenPositionsContract mirrors Python mexc_client.get_open_positions:
// signed GET on contract.mexc.com with an empty JSON object as the sign payload.
func (c *Client) GetOpenPositionsContract(ctx context.Context) ([]byte, error) {
	return c.getSignedContract(ctx, pathOpenPositionsContract, nil, map[string]any{})
}

// GetContractDetailContractPublic mirrors Python contract host GET /contract/detail (unsigned).
func (c *Client) GetContractDetailContractPublic(ctx context.Context, symbol string) ([]byte, error) {
	q := url.Values{}
	q.Set("symbol", symbol)
	return c.getPublicContract(ctx, pathContractDetailPublic, q)
}

// ContractDetailSummary is a small typed slice of contract fields (Python get_contract_detail subset).
type ContractDetailSummary struct {
	ContractSize float64 `json:"contract_size"`
	MaxLeverage  int     `json:"max_leverage"`
	MaxVolume    float64 `json:"max_volume"`
	MinVolume    float64 `json:"min_volume"`
	VolScale     int     `json:"vol_scale"`
	VolUnit      float64 `json:"vol_unit"`
	Data         map[string]any `json:"-"`
}

// ParseContractDetailSummary parses contract host JSON like Python get_contract_detail.
func ParseContractDetailSummary(body []byte) (ContractDetailSummary, error) {
	var outer struct {
		Success bool `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &outer); err != nil {
		return ContractDetailSummary{}, err
	}
	if !outer.Success || len(outer.Data) == 0 {
		return ContractDetailSummary{}, fmt.Errorf("mexcfutures: contract detail: empty or unsuccessful")
	}

	var mid map[string]any
	if err := json.Unmarshal(outer.Data, &mid); err != nil {
		return ContractDetailSummary{}, err
	}

	fields := mid
	if nested, ok := mid["data"].(map[string]any); ok {
		if s, ok := mid["success"].(bool); ok && s {
			fields = nested
		}
	}

	out := ContractDetailSummary{Data: fields}
	out.ContractSize = anyToFloat64(fields["contractSize"])
	out.MaxLeverage = int(anyToFloat64(fields["maxLeverage"]))
	out.MaxVolume = anyToFloat64(fields["maxVol"])
	out.MinVolume = anyToFloat64(fields["minVol"])
	out.VolScale = int(anyToFloat64(fields["volScale"]))
	out.VolUnit = anyToFloat64(fields["volUnit"])
	return out, nil
}

func anyToFloat64(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	default:
		return 0
	}
}
