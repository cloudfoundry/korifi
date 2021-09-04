package apis_test

import (
	"fmt"
)

const (
	defaultServerURL = "https://api.example.org"
	jsonHeader       = "application/json"
)

func defaultServerURI(path string) string {
	return fmt.Sprintf("%s%s", defaultServerURL, path)
}
