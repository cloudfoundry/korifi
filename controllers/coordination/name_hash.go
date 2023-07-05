package coordination

import (
	"crypto/sha1"
	"fmt"
)

const hashedNamePrefix = "n-"

func HashName(entityType, name string) string {
	input := fmt.Sprintf("%s::%s", entityType, name)
	return fmt.Sprintf("%s%x", hashedNamePrefix, sha1.Sum([]byte(input)))
}
