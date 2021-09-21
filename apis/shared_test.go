package apis_test

import (
	"fmt"
	"strings"
)

const (
	defaultServerURL = "https://api.example.org"
	jsonHeader       = "application/json"
)

func defaultServerURI(paths ...string) string {
	return fmt.Sprintf("%s%s", defaultServerURL, strings.Join(paths, ""))
}
