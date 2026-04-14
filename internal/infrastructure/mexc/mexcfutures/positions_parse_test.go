package mexcfutures

import (
	"testing"
)

func TestParseOpenPositionsResponse_docSample(t *testing.T) {
	const sample = `{"success":true,"code":0,"data":[{"positionId":1109973831,"symbol":"BTC_USDT","positionType":1,"openType":1,"state":1,"holdVol":5,"leverage":2}]}`
	rows, err := ParseOpenPositionsResponse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d want 1", len(rows))
	}
	if rows[0].Symbol != "BTC_USDT" || rows[0].HoldVol != 5 || !rows[0].IsLong || rows[0].PositionID != 1109973831 {
		t.Fatalf("unexpected row: %+v", rows[0])
	}
}

func TestParseOpenPositionsResponse_skipsZeroHold(t *testing.T) {
	const sample = `{"success":true,"code":0,"data":[{"positionId":1,"symbol":"BTC_USDT","positionType":1,"holdVol":0,"leverage":2}]}`
	rows, err := ParseOpenPositionsResponse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("want empty, got %+v", rows)
	}
}
