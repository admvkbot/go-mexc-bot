package mexcfutures

import (
	"crypto/md5"
	"encoding/hex"
	"strconv"
	"time"
)

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// webSignature computes x-mxc-nonce / x-mxc-sign for a JSON body identical to
// Python mexc_client and TS generateHeaders (MD5 chain).
func webSignature(webKey string, bodyJSON []byte) (nonce string, sign string) {
	nonce = strconv.FormatInt(time.Now().UnixMilli(), 10)
	g := md5Hex(webKey + nonce)[7:]
	sign = md5Hex(nonce + string(bodyJSON) + g)
	return nonce, sign
}
