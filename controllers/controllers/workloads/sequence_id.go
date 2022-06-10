package workloads

import (
	"strconv"
	"strings"

	"github.com/jonboulle/clockwork"
)

type SequenceId struct {
	clock clockwork.Clock
}

func NewSequenceId(clock clockwork.Clock) *SequenceId {
	return &SequenceId{clock: clock}
}

func (s *SequenceId) Generate() (int64, error) {
	now := s.clock.Now()

	// This is a bit annoying. Golang's builtin support for millis in
	// time.Format() would always put a dot (or comma) before the millis. We do
	// not need them for sequence IDs that we want to be in the format of
	// YYYYMMDDhhmmssSSS
	seqIdString := strings.ReplaceAll(now.Format("20060102150405.000"), ".", "")

	return strconv.ParseInt(seqIdString, 10, 64)
}
