package validators

import (
	"errors"
	"regexp"

	"code.cloudfoundry.org/bytefmt"
	"code.cloudfoundry.org/korifi/api/payloads"
	"github.com/jellydator/validation"
)

type Manifest struct{}

func NewManifest() Manifest {
	return Manifest{}
}

func (m Manifest) ValidatePayload(o any) error {
	manifest := o.(payloads.Manifest)

	return validation.ValidateStruct(&manifest,
		validation.Field(&manifest.Applications, validation.Each(validation.By(validateApplication))),
	)
}

func validateApplication(o any) error {
	a := o.(payloads.ManifestApplication)

	return validation.ValidateStruct(&a,
		validation.Field(&a.Name, validation.Required),
		validation.Field(&a.DefaultRoute, validation.When(a.RandomRoute, validation.Nil.Error("and random-route may not be used together"))),
		validation.Field(&a.DiskQuota, validation.By(validateAmountWithUnit), validation.When(a.AltDiskQuota != nil, validation.Nil.Error("and disk-quota may not be used together"))),
		validation.Field(&a.AltDiskQuota, validation.By(validateAmountWithUnit)),
		validation.Field(&a.Instances, validation.Min(0)),
		validation.Field(&a.HealthCheckInvocationTimeout, validation.Min(1), validation.NilOrNotEmpty.Error("must be no less than 1")),
		validation.Field(&a.HealthCheckType, validation.In("none", "process", "port", "http")),
		validation.Field(&a.Memory, validation.By(validateAmountWithUnit)),
		validation.Field(&a.Timeout, validation.Min(1), validation.NilOrNotEmpty.Error("must be no less than 1")),
		validation.Field(&a.Processes, validation.Each(validation.By(validateProcess))),
		validation.Field(&a.Routes, validation.Each(validation.By(validateRoute))),
	)
}

func validateProcess(o any) error {
	p := o.(payloads.ManifestApplicationProcess)

	return validation.ValidateStruct(&p,
		validation.Field(&p.Type, validation.Required),
		validation.Field(&p.DiskQuota, validation.By(validateAmountWithUnit), validation.When(p.AltDiskQuota != nil, validation.Nil.Error("and disk-quota may not be used together"))),
		validation.Field(&p.AltDiskQuota, validation.By(validateAmountWithUnit)),
		validation.Field(&p.HealthCheckInvocationTimeout, validation.Min(1), validation.NilOrNotEmpty.Error("must be no less than 1")),
		validation.Field(&p.HealthCheckType, validation.In("none", "process", "port", "http")),
		validation.Field(&p.Instances, validation.Min(0)),
		validation.Field(&p.Memory, validation.By(validateAmountWithUnit)),
		validation.Field(&p.Timeout, validation.Min(1), validation.NilOrNotEmpty.Error("must be no less than 1")),
	)
}

func validateRoute(o any) error {
	r := o.(payloads.ManifestRoute)

	routeRegex := regexp.MustCompile(
		`^(?:https?://|tcp://)?(?:(?:[\w-]+\.)|(?:[*]\.))+\w+(?:\:\d+)?(?:/.*)*(?:\.\w+)?$`,
	)

	return validation.ValidateStruct(&r,
		validation.Field(&r.Route, validation.Match(routeRegex).Error("is not a valid route")),
	)
}

var unitAmount = regexp.MustCompile(`^\d+(?:B|K|KB|M|MB|G|GB|T|TB)$`)

func validateAmountWithUnit(value any) error {
	v, isNil := validation.Indirect(value)
	if isNil {
		return nil
	}

	if !unitAmount.MatchString(v.(string)) {
		return errors.New("must use a supported unit (B, K, KB, M, MB, G, GB, T, or TB)")
	}

	mbs, err := bytefmt.ToMegabytes(v.(string))
	if err != nil {
		return err
	}

	if mbs <= 0 {
		return errors.New("must be greater than 0MB")
	}

	return nil
}
