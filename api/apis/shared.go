package apis

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	"code.cloudfoundry.org/bytefmt"
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

func decodeAndValidateJSONPayload(r *http.Request, object interface{}) *requestMalformedError {
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

	return validatePayload(object)
}

func validatePayload(object interface{}) *requestMalformedError {
	v := validator.New()

	trans := registerDefaultTranslator(v)

	// Register custom validators
	_ = v.RegisterValidation("routepathstartswithslash", routePathStartsWithSlash)
	_ = v.RegisterValidation("megabytestring", megabyteFormattedString, true)

	v.RegisterStructValidation(checkRoleTypeAndOrgSpace, payloads.RoleCreate{})
	_ = v.RegisterTranslation("cannot_have_both_org_and_space_set", trans, func(ut ut.Translator) error {
		return ut.Add("cannot_have_both_org_and_space_set", "Cannot pass both 'organization' and 'space' in a create role request", false)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("cannot_have_both_org_and_space_set", fe.Field())
		return t
	})
	_ = v.RegisterTranslation("valid_role", trans, func(ut ut.Translator) error {
		return ut.Add("valid_role", "{0} is not a valid role", false)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("valid_role", fmt.Sprintf("%v", fe.Value()))
		return t
	})

	err := v.Struct(object)
	if err != nil {
		switch typedErr := err.(type) {
		case validator.ValidationErrors:
			errorMap := typedErr.Translate(trans)
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

func registerDefaultTranslator(v *validator.Validate) ut.Translator {
	en := en.New()
	uni := ut.New(en, en)
	trans, _ := uni.GetTranslator("en")
	_ = en_translations.RegisterDefaultTranslations(v, trans)
	return trans
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

	err := json.NewEncoder(w).Encode(responseBody)
	if err != nil {
		Logger.Error(err, "failed to encode and wire response")
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

func megabyteFormattedString(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Interface().(string)
	if !ok {
		return true
	}

	_, err := bytefmt.ToMegabytes(val)
	return err == nil
}
