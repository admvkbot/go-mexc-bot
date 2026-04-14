package mexcfutures

import "testing"

func TestParseContractDetailSummary(t *testing.T) {
	body := []byte(`{"success":true,"data":{"contractSize":0.0001,"maxLeverage":500,"maxVol":400000,"minVol":1,"volScale":0,"volUnit":1}}`)
	s, err := ParseContractDetailSummary(body)
	if err != nil {
		t.Fatal(err)
	}
	if s.ContractSize != 0.0001 || s.MaxLeverage != 500 {
		t.Fatalf("unexpected summary: %+v", s)
	}
}
