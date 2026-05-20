package main

import "encoding/base64"

// base64URLDecodeStd is split into its own file because main.go uses a
// custom alphabet name from go-jwt and we want the std-lib call distinct
// for clarity.
func base64URLDecodeStd(s string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(s)
}
