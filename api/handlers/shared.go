package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"

	"code.cloudfoundry.org/bytefmt"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
	"golang.org/x/exp/maps"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

type DecoderValidator struct {
	validator  *validator.Validate
	translator ut.Translator
}

func NewDefaultDecoderValidator() (*DecoderValidator, error) {
	validator, translator, err := wireValidator()
	if err != nil {
		return nil, err
	}

	return &DecoderValidator{
		validator:  validator,
		translator: translator,
	}, nil
}

func (dv *DecoderValidator) DecodeAndValidateJSONPayload(r *http.Request, object interface{}) error {
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	decoder.DisallowUnknownFields()
	err := decoder.Decode(object)
	if err != nil {
		var unmarshalTypeError *json.UnmarshalTypeError
		switch {
		case errors.As(err, &unmarshalTypeError):
			titler := cases.Title(language.AmericanEnglish)
			return apierrors.NewUnprocessableEntityError(err, fmt.Sprintf("%v must be a %v", titler.String(unmarshalTypeError.Field), unmarshalTypeError.Type))
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			// check whether the message matches an "unknown field" error. If so, 422. Else, 400
			return apierrors.NewUnprocessableEntityError(err, fmt.Sprintf("invalid request body: %s", err.Error()))
		default:
			return apierrors.NewMessageParseError(err)
		}
	}

	return dv.validatePayload(object)
}

func (dv *DecoderValidator) DecodeAndValidateYAMLPayload(r *http.Request, object interface{}) error {
	decoder := yaml.NewDecoder(r.Body)
	defer r.Body.Close()
	decoder.KnownFields(false) // TODO: change this to true once we've added all manifest fields to payloads.Manifest
	err := decoder.Decode(object)
	if err != nil {
		return apierrors.NewMessageParseError(err)
	}

	return dv.validatePayload(object)
}

func (dv *DecoderValidator) validatePayload(object interface{}) error {
	err := dv.validator.Struct(object)
	if err != nil {
		errorMessage := err.Error()

		var typedErr validator.ValidationErrors
		if errors.As(err, &typedErr) {
			errorMap := typedErr.Translate(dv.translator)
			var errorMessages []string
			for _, msg := range errorMap {
				errorMessages = append(errorMessages, msg)
			}

			if len(errorMessages) > 0 {
				errorMessage = strings.Join(errorMessages, ",")
			}
		}
		return apierrors.NewUnprocessableEntityError(err, errorMessage)
	}

	return nil
}

func wireValidator() (*validator.Validate, ut.Translator, error) {
	v := validator.New()

	trans, err := registerDefaultTranslator(v)
	if err != nil {
		return nil, nil, err
	}
	// Register custom validators
	err = v.RegisterValidation("amountWithUnit", amountWithUnit, true)
	if err != nil {
		return nil, nil, err
	}

	err = v.RegisterValidation("positiveAmountWithUnit", positiveAmountWithUnit, true)
	if err != nil {
		return nil, nil, err
	}

	err = v.RegisterValidation("route", routeString)
	if err != nil {
		return nil, nil, err
	}
	err = v.RegisterValidation("serviceinstancetaglength", serviceInstanceTagLength)
	if err != nil {
		return nil, nil, err
	}

	err = v.RegisterValidation("metadatavalidator", metadataValidator)
	if err != nil {
		return nil, nil, err
	}
	err = v.RegisterTranslation("metadatavalidator", trans, func(ut ut.Translator) error {
		return ut.Add("metadatavalidator", `Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`, false)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("metadatavalidator", fe.Field())
		return t
	})
	if err != nil {
		return nil, nil, err
	}

	err = v.RegisterValidation("buildmetadatavalidator", buildMetadataValidator)
	if err != nil {
		return nil, nil, err
	}
	err = v.RegisterTranslation("buildmetadatavalidator", trans, func(ut ut.Translator) error {
		return ut.Add("buildmetadatavalidator", `Labels and annotations are not supported for builds`, false)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("buildmetadatavalidator", fe.Field())
		return t
	})
	if err != nil {
		return nil, nil, err
	}

	v.RegisterStructValidation(validateManifest, payloads.ManifestApplication{})
	v.RegisterStructValidation(checkDiskQuotaUnderscoreAndHyphenProc, payloads.ManifestApplicationProcess{})

	v.RegisterStructValidation(checkRoleTypeAndOrgSpace, payloads.RoleCreate{})

	err = v.RegisterTranslation("amountWithUnit", trans, func(ut ut.Translator) error {
		return ut.Add("amountWithUnit", "{0} must use a supported unit: B, K, KB, M, MB, G, GB, T, or TB", false)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("amountWithUnit", fe.Field())
		return t
	})
	if err != nil {
		return nil, nil, err
	}

	err = v.RegisterTranslation("positiveAmountWithUnit", trans, func(ut ut.Translator) error {
		return ut.Add("positiveAmountWithUnit", "{0} must be greater than 0MB", false)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("positiveAmountWithUnit", fe.Field())
		return t
	})
	if err != nil {
		return nil, nil, err
	}

	err = v.RegisterTranslation("cannot_have_both_org_and_space_set", trans, func(ut ut.Translator) error {
		return ut.Add("cannot_have_both_org_and_space_set", "Cannot pass both 'organization' and 'space' in a create role request", false)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("cannot_have_both_org_and_space_set", fe.Field())
		return t
	})
	if err != nil {
		return nil, nil, err
	}

	err = v.RegisterTranslation("valid_role", trans, func(ut ut.Translator) error {
		return ut.Add("valid_role", "{0} is not a valid role", false)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("valid_role", fmt.Sprintf("%v", fe.Value()))
		return t
	})
	if err != nil {
		return nil, nil, err
	}

	err = v.RegisterTranslation("route", trans, func(ut ut.Translator) error {
		return ut.Add("invalid_route", `"{0}" is not a valid route URI`, false)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("invalid_route", fmt.Sprintf("%v", fe.Value()))
		return t
	})
	if err != nil {
		return nil, nil, err
	}

	err = v.RegisterTranslation("both-disk-quotas-set", trans, func(ut ut.Translator) error {
		return ut.Add("both-disk-quotas-set", "Cannot set both 'disk-quota' and 'disk_quota' in manifest", false)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("both-disk-quotas-set", fe.Field())
		return t
	})
	if err != nil {
		return nil, nil, err
	}

	return v, trans, nil
}

func registerDefaultTranslator(v *validator.Validate) (ut.Translator, error) {
	en := en.New()
	uni := ut.New(en, en)
	trans, _ := uni.GetTranslator("en")

	err := en_translations.RegisterDefaultTranslations(v, trans)
	if err != nil {
		return nil, err
	}

	return trans, nil
}

func validateManifest(sl validator.StructLevel) {
	manifestApplication := sl.Current().Interface().(payloads.ManifestApplication)
	checkRandomRouteAndDefaultRouteConflict(manifestApplication, sl)
	checkDiskQuotaUnderscoreAndHyphenApp(manifestApplication, sl)
}

func checkRandomRouteAndDefaultRouteConflict(manifestApplication payloads.ManifestApplication, sl validator.StructLevel) {
	if manifestApplication.DefaultRoute && manifestApplication.RandomRoute {
		sl.ReportError(manifestApplication.DefaultRoute, "defaultRoute", "DefaultRoute", "Random-route and Default-route may not be used together", "")
	}
}

func checkDiskQuotaUnderscoreAndHyphenApp(manifestApplication payloads.ManifestApplication, sl validator.StructLevel) {
	//nolint:staticcheck
	if manifestApplication.DiskQuota != nil && manifestApplication.AltDiskQuota != nil {
		//nolint:staticcheck
		sl.ReportError(manifestApplication.AltDiskQuota, "disk-quota", "AltDiskQuota", "both-disk-quotas-set", "")
	}
}

func checkDiskQuotaUnderscoreAndHyphenProc(sl validator.StructLevel) {
	manifestProcess := sl.Current().Interface().(payloads.ManifestApplicationProcess)

	//nolint:staticcheck
	if manifestProcess.DiskQuota != nil && manifestProcess.AltDiskQuota != nil {
		sl.ReportError(manifestProcess.AltDiskQuota, "disk-quota", "AltDiskQuota", "both-disk-quotas-set", "")
	}
}

func checkRoleTypeAndOrgSpace(sl validator.StructLevel) {
	roleCreate := sl.Current().Interface().(payloads.RoleCreate)

	if roleCreate.Relationships.Organization != nil && roleCreate.Relationships.Space != nil {
		sl.ReportError(roleCreate.Relationships.Organization, "relationships.organization", "Organization", "cannot_have_both_org_and_space_set", "")
	}

	roleType := RoleName(roleCreate.Type)

	switch roleType {
	case RoleSpaceManager:
		fallthrough
	case RoleSpaceAuditor:
		fallthrough
	case RoleSpaceDeveloper:
		fallthrough
	case RoleSpaceSupporter:
		if roleCreate.Relationships.Space == nil {
			sl.ReportError(roleCreate.Relationships.Space, "relationships.space", "Space", "required", "")
		}
	case RoleOrganizationUser:
		fallthrough
	case RoleOrganizationAuditor:
		fallthrough
	case RoleOrganizationManager:
		fallthrough
	case RoleOrganizationBillingManager:
		if roleCreate.Relationships.Organization == nil {
			sl.ReportError(roleCreate.Relationships.Organization, "relationships.organization", "Organization", "required", "")
		}

	case RoleName(""):
	default:
		sl.ReportError(roleCreate.Type, "type", "Role type", "valid_role", "")
	}
}

var unitAmount = regexp.MustCompile(`^\d+(?:B|K|KB|M|MB|G|GB|T|TB)$`)

func amountWithUnit(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Interface().(string)
	if !ok {
		return true // the value is optional, and is set to nil
	}

	return unitAmount.MatchString(val)
}

func positiveAmountWithUnit(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Interface().(string)
	if !ok {
		return true // the value is optional, and is set to nil
	}

	mbs, err := bytefmt.ToMegabytes(val)
	return err == nil && mbs > 0
}

func routeString(fl validator.FieldLevel) bool {
	val := fl.Field().String()
	routeRegex := regexp.MustCompile(
		`^(?:https?://|tcp://)?(?:(?:[\w-]+\.)|(?:[*]\.))+\w+(?:\:\d+)?(?:/.*)*(?:\.\w+)?$`,
	)
	return routeRegex.MatchString(val)
}

func serviceInstanceTagLength(fl validator.FieldLevel) bool {
	tags, ok := fl.Field().Interface().([]string)
	if !ok {
		return true // the value is optional, and is set to nil
	}

	tagLen := 0
	for _, tag := range tags {
		tagLen += len(tag)
	}

	return tagLen < 2048
}

func metadataValidator(fl validator.FieldLevel) bool {
	metadata, isMeta := fl.Field().Interface().(map[string]string)
	if isMeta {
		return validateMetadataKeys(maps.Keys(metadata))
	}

	metadataPatch, isMetaPatch := fl.Field().Interface().(map[string]*string)
	if isMetaPatch {
		return validateMetadataKeys(maps.Keys(metadataPatch))
	}

	return true
}

func buildMetadataValidator(fl validator.FieldLevel) bool {
	metadata, isMeta := fl.Field().Interface().(map[string]string)
	if isMeta {
		if len(metadata) > 0 {
			return false
		}
	}
	return true
}

func validateMetadataKeys(metaKeys []string) bool {
	for _, key := range metaKeys {
		u, err := url.ParseRequestURI("https://" + key) // without the scheme, the hostname will be parsed as a path
		if err != nil {
			continue
		}

		if strings.HasSuffix(u.Hostname(), "cloudfoundry.org") {
			return false
		}
	}

	return true
}

type Presenter[T, S any] interface {
	PresentResource(T) S
	PresentList([]T, url.URL) presenter.ResourcesResponse[S]
}
