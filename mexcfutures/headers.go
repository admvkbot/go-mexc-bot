package mexcfutures

import "net/http"

func (c *Client) applyDefaultHeaders(h http.Header) {
	h.Set("accept", "*/*")
	h.Set("accept-language", "en-US,en;q=0.9,ru;q=0.8")
	h.Set("cache-control", "no-cache")
	h.Set("content-type", "application/json")
	h.Set("dnt", "1")
	h.Set("origin", "https://www.mexc.com")
	h.Set("pragma", "no-cache")
	h.Set("referer", "https://www.mexc.com/")
	h.Set("user-agent", c.userAgent)
	h.Set("x-language", "en-US")
}

func (c *Client) applyAuthHeader(h http.Header) {
	h.Set("authorization", c.webKey)
}

func (c *Client) applySignedPOST(h http.Header, bodyJSON []byte) {
	nonce, sign := webSignature(c.webKey, bodyJSON)
	h.Set("x-mxc-nonce", nonce)
	h.Set("x-mxc-sign", sign)
}
