package scalper

import (
	"strings"
	"testing"
)

func TestFuturesSubmitAPIError_successFalse(t *testing.T) {
	raw := []byte(`{"success":false,"code":2005,"message":"Balance insufficient","_extend":{"cost":"2.44","available":"0.31"}}`)
	err := futuresSubmitAPIError(raw)
	if err == nil {
		t.Fatal("expected error")
	}
	s := err.Error()
	if !strings.Contains(s, "2005") || !strings.Contains(s, "Balance insufficient") {
		t.Fatalf("unexpected message: %s", s)
	}
	if !strings.Contains(s, "cost") {
		t.Fatalf("expected _extend in error: %s", s)
	}
}

func TestFuturesSubmitAPIError_successTrue(t *testing.T) {
	raw := []byte(`{"success":true,"data":{"orderId":"123"}}`)
	if futuresSubmitAPIError(raw) != nil {
		t.Fatal("expected nil")
	}
}

func TestFuturesSubmitAPIError_successOmitted(t *testing.T) {
	raw := []byte(`{"code":0,"data":{"orderId":"x"}}`)
	if futuresSubmitAPIError(raw) != nil {
		t.Fatal("omitted success must not be treated as failure")
	}
}
