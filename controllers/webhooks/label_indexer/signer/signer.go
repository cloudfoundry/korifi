package signer

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

const ReservedLabelDomain = "korifi.cloudfoundry.org/"

func Sign(secret []byte, labels map[string]string) string {
	mac := hmac.New(sha256.New, secret)
	for _, k := range sortedReservedKeys(labels) {
		mac.Write([]byte(k))
		mac.Write([]byte("="))
		mac.Write([]byte(labels[k]))
		mac.Write([]byte("\n"))
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func Verify(secret []byte, labels map[string]string, sig string) bool {
	return hmac.Equal([]byte(Sign(secret, labels)), []byte(sig))
}

func sortedReservedKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		if strings.HasPrefix(k, ReservedLabelDomain) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}
