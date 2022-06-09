package prometheus_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/korifi/statefulset-runner/prometheus"
	"code.cloudfoundry.org/korifi/statefulset-runner/prometheus/prometheusfakes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	prometheus_api "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	clock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var _ = Describe("LRP Desirer Prometheus Decorator", func() {
	var (
		desirer   *prometheusfakes.FakeLRPDesirer
		decorator prometheus.LRPDesirer
		lrp       *eiriniv1.LRP
		desireErr error
		registry  metrics.RegistererGatherer
		fakeClock *clock.FakePassiveClock
		t0        time.Time
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		desirer = new(prometheusfakes.FakeLRPDesirer)
		lrp = &eiriniv1.LRP{}
		registry = prometheus_api.NewRegistry()

		t0 = time.Now()
		fakeClock = clock.NewFakePassiveClock(t0)

		var err error
		decorator, err = prometheus.NewLRPDesirerDecorator(desirer, registry, fakeClock)
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		desireErr = decorator.Desire(ctx, lrp)
	})

	It("succeeds", func() {
		Expect(desireErr).NotTo(HaveOccurred())
	})

	It("delegates to the LRP client on Desire", func() {
		Expect(desirer.DesireCallCount()).To(Equal(1))
		_, actualLrp := desirer.DesireArgsForCall(0)
		Expect(actualLrp).To(Equal(lrp))
	})

	It("increments the LRP creation counter", func() {
		Expect(registry).To(HaveCounter(prometheus.LRPCreations, prometheus.LRPCreationsHelp, 1))
	})

	Describe("observing durations", func() {
		BeforeEach(func() {
			desirer.DesireStub = func(ctx context.Context, l *eiriniv1.LRP) error {
				fakeClock.SetTime(t0.Add(time.Second))

				return nil
			}
		})

		It("measures the duration of the desiring", func() {
			Expect(registry).To(HaveHistogram(prometheus.LRPCreationDurations, prometheus.LRPCreationDurationsHelp, 1000, 1))
		})
	})

	When("desiring the lrp fails", func() {
		BeforeEach(func() {
			desirer.DesireReturns(errors.New("foo"))
		})

		It("fails", func() {
			Expect(desireErr).To(MatchError("foo"))
		})

		It("does not increment the LRP creation counter", func() {
			Expect(registry).To(HaveCounter(prometheus.LRPCreations, prometheus.LRPCreationsHelp, 0))
		})

		It("does not measure the duration of the desiring", func() {
			Expect(registry).To(HaveHistogram(prometheus.LRPCreationDurations, prometheus.LRPCreationDurationsHelp, 0, 0))
		})
	})

	When("using a shared registry", func() {
		var otherDecorator *prometheus.LRPDesirerDecorator

		BeforeEach(func() {
			var err error
			otherDecorator, err = prometheus.NewLRPDesirerDecorator(desirer, registry, fakeClock)
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			Expect(otherDecorator.Desire(ctx, lrp)).To(Succeed())
		})

		It("adopts the existing counters", func() {
			Expect(registry).To(HaveCounter(prometheus.LRPCreations, prometheus.LRPCreationsHelp, 2))
		})
	})
})

func HaveMetric(name string, promText string) types.GomegaMatcher {
	return WithTransform(func(registry prometheus_api.Gatherer) error {
		return testutil.GatherAndCompare(registry, strings.NewReader(promText), name)
	}, Succeed())
}

func HaveCounter(name, help string, value int) types.GomegaMatcher {
	return HaveMetric(name, fmt.Sprintf(`
		# HELP %[1]s %[2]s
		# TYPE %[1]s counter
		%[1]s %[3]d
		`,
		name, help, value,
	))
}

func HaveHistogram(name, help string, sum float64, count int) types.GomegaMatcher {
	return HaveMetric(name, fmt.Sprintf(`
		# HELP %[1]s %[2]s
		# TYPE %[1]s histogram
		%[1]s_sum %[3]f
		%[1]s_count %[4]d
		%[1]s_bucket{le="0.005"} 0
		%[1]s_bucket{le="0.01"} 0
		%[1]s_bucket{le="0.025"} 0
		%[1]s_bucket{le="0.05"} 0
		%[1]s_bucket{le="0.1"} 0
		%[1]s_bucket{le="0.25"} 0
		%[1]s_bucket{le="0.5"} 0
		%[1]s_bucket{le="1"} 0
		%[1]s_bucket{le="2.5"} 0
		%[1]s_bucket{le="5"} 0
		%[1]s_bucket{le="10"} 0
		`,
		name, help, sum, count,
	))
}
