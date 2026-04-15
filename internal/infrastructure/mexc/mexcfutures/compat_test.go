package mexcfutures

import "testing"

func TestParseContractDetailSummary(t *testing.T) {
	body := []byte(`{"success":true,"data":{"contractSize":0.0001,"maxLeverage":500,"maxVol":400000,"minVol":1,"priceScale":2,"volScale":0,"volUnit":1}}`)
	s, err := ParseContractDetailSummary(body)
	if err != nil {
		t.Fatal(err)
	}
	if s.ContractSize != 0.0001 || s.MaxLeverage != 500 || s.PriceScale != 2 || s.VolScale != 0 {
		t.Fatalf("unexpected summary: %+v", s)
	}
}
