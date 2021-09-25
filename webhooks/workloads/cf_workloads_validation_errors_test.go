package workloads_test

import (
	. "code.cloudfoundry.org/cf-k8s-controllers/webhooks/workloads"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"testing"
)

func TestCFWorkloadsValidationErrors(t *testing.T) {
	spec.Run(t, "CFWorkload Errors lib", testCFWorkloadsErrors, spec.Report(report.Terminal{}))

}

func testCFWorkloadsErrors(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	it("Marshals a payload", func() {
		e := DuplicateAppError
		g.Expect(e.Marshal()).To(Equal(`{"code":1,"message":"CFApp with the same spec.name exists"}`))
	})

	it("Unmarshals UnknownError", func() {
		e := new(ValidationErrorCode)
		p := `{"code":0}`
		e.Unmarshall(p)
		g.Expect(*e).To(Equal(UnknownError))
	})

	it("Unmarshals DuplicateAppError", func() {
		e := new(ValidationErrorCode)
		p := `{"code":1}`
		e.Unmarshall(p)
		g.Expect(*e).To(Equal(DuplicateAppError))
	})

	it("Handles malformed json payloads", func() {
		e := new(ValidationErrorCode)
		p := `{"code":1`
		e.Unmarshall(p)
		g.Expect(*e).To(Equal(UnknownError))
	})
}
