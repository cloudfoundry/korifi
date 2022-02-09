package apis

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"code.cloudfoundry.org/bytefmt"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"gopkg.in/yaml.v3"

	"github.com/go-logr/logr"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
	ctrl "sigs.k8s.io/controller-runtime"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

var Logger = ctrl.Log.WithName("Shared Handler Functions")

type requestMalformedError struct {
	httpStatus    int
	errorResponse presenter.ErrorsResponse
}

func (rme *requestMalformedError) Error() string {
	return fmt.Sprintf("Error throwing an http %v", rme.httpStatus)
}

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

func (dv *DecoderValidator) DecodeAndValidateJSONPayload(r *http.Request, object interface{}) *requestMalformedError {
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	decoder.DisallowUnknownFields()
	err := decoder.Decode(object)
	if err != nil {
		var unmarshalTypeError *json.UnmarshalTypeError
		switch {
		case errors.As(err, &unmarshalTypeError):
			Logger.Error(err, fmt.Sprintf("Request body contains an invalid value for the %q field (should be of type %v)", strings.Title(unmarshalTypeError.Field), unmarshalTypeError.Type))
			return &requestMalformedError{
				httpStatus:    http.StatusUnprocessableEntity,
				errorResponse: newUnprocessableEntityError(fmt.Sprintf("%v must be a %v", strings.Title(unmarshalTypeError.Field), unmarshalTypeError.Type)),
			}
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			// check whether the message matches an "unknown field" error. If so, 422. Else, 400
			Logger.Error(err, fmt.Sprintf("Unknown field in JSON body: %T: %q", err, err.Error()))
			return &requestMalformedError{
				httpStatus:    http.StatusUnprocessableEntity,
				errorResponse: newUnprocessableEntityError(fmt.Sprintf("invalid request body: %s", err.Error())),
			}
		default:
			Logger.Error(err, fmt.Sprintf("Unable to parse the JSON body: %T: %q", err, err.Error()))
			return &requestMalformedError{
				httpStatus:    http.StatusBadRequest,
				errorResponse: newMessageParseError(),
			}
		}
	}

	return dv.validatePayload(object)
}

func (dv *DecoderValidator) DecodeAndValidateYAMLPayload(r *http.Request, object interface{}) *requestMalformedError {
	decoder := yaml.NewDecoder(r.Body)
	defer r.Body.Close()
	decoder.KnownFields(false) // TODO: change this to true once we've added all manifest fields to payloads.Manifest
	err := decoder.Decode(object)
	if err != nil {
		Logger.Error(err, fmt.Sprintf("Unable to parse the YAML body: %T: %q", err, err.Error()))
		return &requestMalformedError{
			httpStatus:    http.StatusBadRequest,
			errorResponse: newMessageParseError(),
		}
	}

	return dv.validatePayload(object)
}

func (dv *DecoderValidator) validatePayload(object interface{}) *requestMalformedError {
	err := dv.validator.Struct(object)
	if err != nil {
		switch typedErr := err.(type) {
		case validator.ValidationErrors:
			errorMap := typedErr.Translate(dv.translator)
			var errorMessages []string
			for _, msg := range errorMap {
				errorMessages = append(errorMessages, msg)
			}

			if len(errorMessages) > 0 {
				return &requestMalformedError{
					httpStatus:    http.StatusUnprocessableEntity,
					errorResponse: newUnprocessableEntityError(strings.Join(errorMessages, ",")),
				}
			}
		default:
			return &requestMalformedError{
				httpStatus:    http.StatusUnprocessableEntity,
				errorResponse: newUnprocessableEntityError(err.Error()),
			}
		}
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
	err = v.RegisterValidation("megabytestring", megabyteFormattedString, true)
	if err != nil {
		return nil, nil, err
	}

	err = v.RegisterValidation("route", routeString)
	if err != nil {
		return nil, nil, err
	}
	err = v.RegisterValidation("routepathstartswithslash", routePathStartsWithSlash)
	if err != nil {
		return nil, nil, err
	}
	err = v.RegisterValidation("serviceinstancetaglength", serviceInstanceTagLength)
	if err != nil {
		return nil, nil, err
	}

	v.RegisterStructValidation(checkRoleTypeAndOrgSpace, payloads.RoleCreate{})
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

func newMessageParseError() presenter.ErrorsResponse {
	return presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-MessageParseError",
		Detail: "Request invalid due to parse error: invalid request body",
		Code:   1001,
	}}}
}

func newUnprocessableEntityError(detail string) presenter.ErrorsResponse {
	return presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-UnprocessableEntity",
		Detail: detail,
		Code:   10008,
	}}}
}

func writeNotFoundErrorResponse(w http.ResponseWriter, resourceName string) {
	response := presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-ResourceNotFound",
		Detail: fmt.Sprintf("%s not found", resourceName),
		Code:   10010,
	}}}
	writeResponse(w, http.StatusNotFound, response)
}

func writeUnknownErrorResponse(w http.ResponseWriter) {
	response := presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "UnknownError",
		Detail: "An unknown error occurred.",
		Code:   10001,
	}}}
	writeResponse(w, http.StatusInternalServerError, response)
}

func writeNotAuthenticatedErrorResponse(w http.ResponseWriter) {
	response := presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-NotAuthenticated",
		Detail: "Authentication error",
		Code:   10002,
	}}}
	writeResponse(w, http.StatusUnauthorized, response)
}

func writeNotAuthorizedErrorResponse(w http.ResponseWriter) {
	response := presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-NotAuthorized",
		Detail: "You are not authorized to perform the requested action",
		Code:   10003,
	}}}
	writeResponse(w, http.StatusForbidden, response)
}

func writeInvalidAuthErrorResponse(w http.ResponseWriter) {
	response := presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-InvalidAuthToken",
		Detail: "Invalid Auth Token",
		Code:   1000,
	}}}
	writeResponse(w, http.StatusUnauthorized, response)
}

func writeRequestMalformedErrorResponse(w http.ResponseWriter, rme *requestMalformedError) {
	writeResponse(w, rme.httpStatus, rme.errorResponse)
}

func writeUnprocessableEntityError(w http.ResponseWriter, detail string) {
	writeResponse(w, http.StatusUnprocessableEntity, newUnprocessableEntityError(detail))
}

func writeUniquenessError(w http.ResponseWriter, detail string) {
	response := presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-UniquenessError",
		Detail: detail,
		Code:   10016,
	}}}
	writeResponse(w, http.StatusUnprocessableEntity, response)
}

func writeInvalidRequestError(w http.ResponseWriter, detail string) {
	response := presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-InvalidRequest",
		Detail: detail,
		Code:   10004,
	}}}
	writeResponse(w, http.StatusBadRequest, response)
}

func writePackageBitsAlreadyUploadedError(w http.ResponseWriter) {
	response := presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-PackageBitsAlreadyUploaded",
		Detail: "Bits may be uploaded only once. Create a new package to upload different bits.",
		Code:   150004,
	}}}
	writeResponse(w, http.StatusBadRequest, response)
}

func writeUnknownKeyError(w http.ResponseWriter, validKeys []string) {
	detailMsg := fmt.Sprintf("The query parameter is invalid: Valid parameters are: '%s'", strings.Join(validKeys, ", "))
	response := presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-BadQueryParameter",
		Detail: detailMsg,
		Code:   10005,
	}}}
	writeResponse(w, http.StatusBadRequest, response)
}

func writeResponse(w http.ResponseWriter, status int, responseBody interface{}) {
	w.WriteHeader(status)

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)

	err := encoder.Encode(responseBody)
	if err != nil {
		Logger.Error(err, "failed to encode and write response")
		return
	}
}

// Custom field validators
func routePathStartsWithSlash(fl validator.FieldLevel) bool {
	if fl.Field().String() == "" {
		return true
	}

	if fl.Field().String()[0:1] != "/" {
		return false
	}

	return true
}

func megabyteFormattedString(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Interface().(string)
	if !ok {
		return true // the value is optional, and is set to nil
	}

	_, err := bytefmt.ToMegabytes(val)
	return err == nil
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

func handleRepoErrors(logger logr.Logger, err error, resource, guid string, w http.ResponseWriter) {
	switch err.(type) {
	case repositories.NotFoundError:
		logger.Info(fmt.Sprintf("%s not found", strings.Title(resource)), "guid", guid)
		writeNotFoundErrorResponse(w, strings.Title(resource))
	case repositories.ForbiddenError:
		logger.Info(fmt.Sprintf("%s forbidden to user", strings.Title(resource)), "guid", guid)
		writeNotFoundErrorResponse(w, strings.Title(resource))
	default:
		logger.Error(err, fmt.Sprintf("Failed to fetch %s from Kubernetes", resource), "guid", guid)
		writeUnknownErrorResponse(w)
	}
}
