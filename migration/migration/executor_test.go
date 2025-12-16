package migration_test

import (
	"code.cloudfoundry.org/korifi/migration/migration"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Executor", func() {
	var (
		migrator     *migration.Migrator
		migrationErr error
	)

	BeforeEach(func() {
		migrator = migration.New(k8sClient, "test-version", 1)
	})

	JustBeforeEach(func() {
		migrationErr = migrator.Run(ctx)
	})

	It("is noop", func() {
		Expect(migrationErr).NotTo(HaveOccurred())
	})
})
