package helpers

import (
	"github.com/gofrs/flock"
	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
)

type FLock struct {
	lock *flock.Flock
}

func NewFlock(lockFilePath string) *FLock {
	return &FLock{
		lock: flock.New(lockFilePath),
	}
}

func (f *FLock) Execute(fn func()) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		ok, err := f.lock.TryLock()
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ok).To(BeTrue())
	}).Should(Succeed())

	defer func() {
		Expect(f.lock.Unlock()).To(Succeed())
	}()

	fn()
}
