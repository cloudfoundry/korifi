package diff

import (
	"strings"

	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type Reporter struct {
	path  cmp.Path
	diffs []string
}

func (r *Reporter) PushStep(pushStep cmp.PathStep) {
	r.path = append(r.path, pushStep)
}

func (r *Reporter) Report(compareResult cmp.Result) {
	if !compareResult.Equal() {
		r.diffs = append(r.diffs, r.path.String())
	}
}

func (r *Reporter) PopStep() {
	r.path = r.path[:len(r.path)-1]
}

func (r *Reporter) String() string {
	return strings.Join(unique(r.diffs), ", ")
}

func unique(nonUniqueSlice []string) []string {
	uniqueSlice := []string{}

	for _, elem := range nonUniqueSlice {
		uniqueSlice = appendIfMissing(uniqueSlice, elem)
	}

	return uniqueSlice
}

func appendIfMissing(slice []string, newElem string) []string {
	for _, elem := range slice {
		if elem == newElem {
			return slice
		}
	}

	return append(slice, newElem)
}

func CompareLRPSpecs(updatedLRPSpec, originalLRPSpec *eiriniv1.LRPSpec, ignoredFields ...string) string {
	var reporter Reporter

	ignoreOpts := cmpopts.IgnoreFields(eiriniv1.LRPSpec{}, ignoredFields...)
	equal := cmp.Equal(updatedLRPSpec, originalLRPSpec, cmp.Reporter(&reporter), ignoreOpts)

	if !equal {
		return reporter.String()
	}

	return ""
}
