package helpers

import (
	"github.com/google/uuid"
	. "github.com/onsi/gomega"
)

func EnsureValidUUID(s string) {
	_, err := uuid.Parse(s)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "%s is not a valid UUID", s)
}
