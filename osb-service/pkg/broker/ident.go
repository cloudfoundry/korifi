package broker

import (
	"crypto/rand"
	"regexp"
	"strings"
)

var identBody = regexp.MustCompile(`^[a-z0-9]+$`)

func resourceName(prefix, id string) (string, error) {
	cleaned := strings.ToLower(strings.ReplaceAll(id, "-", ""))
	if !identBody.MatchString(cleaned) {
		return "", BadRequest("instance or binding id must be alphanumeric")
	}
	name := prefix + cleaned
	if len(name) > 63 {
		return "", BadRequest("identifier too long")
	}
	return name, nil
}

func randomPassword(n int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf), nil
}
