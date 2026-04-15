package scalper

import "testing"

func TestParseSubmitOrderID_variants(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"envelope_object", `{"success":true,"data":{"orderId":"720733527158642176"}}`, "720733527158642176"},
		{"envelope_number", `{"success":true,"data":12345}`, "12345"},
		{"top_level", `{"success":true,"orderId":"111","data":{}}`, "111"},
		{"scalar_data_string", `{"data":"999"}`, "999"},
		{"snake_inner", `{"success":true,"data":{"order_id":"42"}}`, "42"},
		{"data_array", `{"success":true,"data":[{"orderId":"7"}]}`, "7"},
		{"data_object_numeric_id", `{"success":true,"data":{"orderId":739106551624717312}}`, "739106551624717312"},
		{"data_json_string", `{"success":true,"data":"{\"orderId\":\"99\"}"}`, "99"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSubmitOrderID([]byte(tc.raw))
			if got != tc.want {
				t.Fatalf("parseSubmitOrderID: got %q want %q", got, tc.want)
			}
		})
	}
}
