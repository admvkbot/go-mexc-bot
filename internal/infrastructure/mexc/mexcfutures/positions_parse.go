package mexcfutures

import (
	"encoding/json"
	"fmt"
)

// OpenPositionClose describes one open futures row from GET /private/position/open_positions
// (see MEXC docs: positionType 1 long, 2 short; holdVol is position size).
type OpenPositionClose struct {
	PositionID   int64
	Symbol       string
	HoldVol      float64
	IsLong       bool
	OpenType     int
	Leverage     int
	PositionType int
}

// ParseOpenPositionsResponse decodes the open_positions JSON envelope and returns rows with holdVol > 0.
func ParseOpenPositionsResponse(body []byte) ([]OpenPositionClose, error) {
	var outer struct {
		Success bool            `json:"success"`
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &outer); err != nil {
		return nil, fmt.Errorf("mexcfutures: open_positions: %w", err)
	}
	if !outer.Success {
		return nil, fmt.Errorf("mexcfutures: open_positions: success=false code=%d msg=%q", outer.Code, outer.Message)
	}
	if len(outer.Data) == 0 || string(outer.Data) == "null" {
		return nil, nil
	}
	var rows []struct {
		PositionID   int64   `json:"positionId"`
		Symbol       string  `json:"symbol"`
		PositionType int     `json:"positionType"`
		HoldVol      float64 `json:"holdVol"`
		OpenType     int     `json:"openType"`
		Leverage     int     `json:"leverage"`
	}
	if err := json.Unmarshal(outer.Data, &rows); err != nil {
		return nil, fmt.Errorf("mexcfutures: open_positions data: %w", err)
	}
	out := make([]OpenPositionClose, 0, len(rows))
	for _, r := range rows {
		if r.HoldVol <= 0 || r.Symbol == "" {
			continue
		}
		isLong := r.PositionType == PositionTypeLong
		if r.PositionType != PositionTypeLong && r.PositionType != PositionTypeShort {
			continue
		}
		out = append(out, OpenPositionClose{
			PositionID:   r.PositionID,
			Symbol:       r.Symbol,
			HoldVol:      r.HoldVol,
			IsLong:       isLong,
			OpenType:     r.OpenType,
			Leverage:     r.Leverage,
			PositionType: r.PositionType,
		})
	}
	return out, nil
}

// Position direction constants from MEXC open position rows (positionType).
const (
	PositionTypeLong  = 1
	PositionTypeShort = 2
)
