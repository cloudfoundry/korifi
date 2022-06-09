package prometheus

import (
	"context"
	"errors"

	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	prometheus_api "github.com/prometheus/client_golang/prometheus"
	"k8s.io/utils/clock"
)

const (
	LRPCreations             = "eirini_lrp_creations"
	LRPCreationsHelp         = "The total number of created lrps"
	LRPCreationDurations     = "eirini_lrp_creation_durations"
	LRPCreationDurationsHelp = "The duration of lrp creations"
)

//counterfeiter:generate . LRPDesirer

type LRPDesirer interface {
	Desire(ctx context.Context, lrp *eiriniv1.LRP) error
}

type LRPDesirerDecorator struct {
	LRPDesirer
	creations         prometheus_api.Counter
	creationDurations prometheus_api.Histogram
	clock             clock.PassiveClock
}

func NewLRPDesirerDecorator(
	desirer LRPDesirer,
	registry prometheus_api.Registerer,
	clck clock.PassiveClock,
) (*LRPDesirerDecorator, error) {
	creations, err := registerCounter(registry, LRPCreations, "The total number of created lrps")
	if err != nil {
		return nil, err
	}

	creationDurations, err := registerHistogram(registry, LRPCreationDurations, LRPCreationDurationsHelp)
	if err != nil {
		return nil, err
	}

	return &LRPDesirerDecorator{
		LRPDesirer:        desirer,
		creations:         creations,
		creationDurations: creationDurations,
		clock:             clck,
	}, nil
}

func (d *LRPDesirerDecorator) Desire(ctx context.Context, lrp *eiriniv1.LRP) error {
	start := d.clock.Now()

	err := d.LRPDesirer.Desire(ctx, lrp)
	if err == nil {
		d.creations.Inc()
		d.creationDurations.Observe(float64(d.clock.Since(start).Milliseconds()))
	}

	return err
}

func registerCounter(registry prometheus_api.Registerer, name, help string) (prometheus_api.Counter, error) {
	c := prometheus_api.NewCounter(prometheus_api.CounterOpts{
		Name: name,
		Help: help,
	})

	err := registry.Register(c)
	if err == nil {
		return c, nil
	}

	var are prometheus_api.AlreadyRegisteredError
	if errors.As(err, &are) {
		return are.ExistingCollector.(prometheus_api.Counter), nil //nolint:forcetypeassert
	}

	return nil, err
}

func registerHistogram(registry prometheus_api.Registerer, name, help string) (prometheus_api.Histogram, error) {
	h := prometheus_api.NewHistogram(prometheus_api.HistogramOpts{
		Name: name,
		Help: help,
	})

	err := registry.Register(h)
	if err == nil {
		return h, nil
	}

	var are prometheus_api.AlreadyRegisteredError
	if errors.As(err, &are) {
		return are.ExistingCollector.(prometheus_api.Histogram), nil //nolint:forcetypeassert
	}

	return nil, err
}
