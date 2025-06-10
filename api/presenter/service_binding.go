package presenter

import (
	"net/url"
	"slices"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
)

const (
	serviceCredentialBindingsBase = "/v3/service_credential_bindings"
)

type ServiceBindingResponse struct {
	GUID          string                              `json:"guid"`
	Type          string                              `json:"type"`
	Name          *string                             `json:"name"`
	CreatedAt     string                              `json:"created_at"`
	UpdatedAt     string                              `json:"updated_at"`
	LastOperation ServiceBindingLastOperationResponse `json:"last_operation"`
	Relationships map[string]ToOneRelationship        `json:"relationships"`
	Links         ServiceBindingLinks                 `json:"links"`
	Metadata      Metadata                            `json:"metadata"`
}

type ServiceBindingLastOperationResponse struct {
	Type        string  `json:"type"`
	State       string  `json:"state"`
	Description *string `json:"description"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type ServiceBindingLinks struct {
	App             Link `json:"app"`
	ServiceInstance Link `json:"service_instance"`
	Self            Link `json:"self"`
	Details         Link `json:"details"`
}

type ServiceBindingDetailsResponse struct {
	Credentials map[string]any `json:"credentials"`
}

func ForServiceBinding(record repositories.ServiceBindingRecord, baseURL url.URL, includes ...include.Resource) ServiceBindingResponse {
	return ServiceBindingResponse{
		GUID:      record.GUID,
		Type:      record.Type,
		Name:      record.Name,
		CreatedAt: tools.ZeroIfNil(formatTimestamp(&record.CreatedAt)),
		UpdatedAt: tools.ZeroIfNil(formatTimestamp(record.UpdatedAt)),
		LastOperation: ServiceBindingLastOperationResponse{
			Type:        record.LastOperation.Type,
			State:       record.LastOperation.State,
			Description: record.LastOperation.Description,
			CreatedAt:   tools.ZeroIfNil(formatTimestamp(&record.LastOperation.CreatedAt)),
			UpdatedAt:   tools.ZeroIfNil(formatTimestamp(record.LastOperation.UpdatedAt)),
		},
		Relationships: ForRelationships(record.Relationships()),
		Links: ServiceBindingLinks{
			App: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, record.AppGUID).build(),
			},
			ServiceInstance: Link{
				HRef: buildURL(baseURL).appendPath(serviceInstancesBase, record.ServiceInstanceGUID).build(),
			},
			Self: Link{
				HRef: buildURL(baseURL).appendPath(serviceCredentialBindingsBase, record.GUID).build(),
			},
			Details: Link{
				HRef: buildURL(baseURL).appendPath(serviceCredentialBindingsBase, record.GUID, "details").build(),
			},
		},
		Metadata: Metadata{
			Labels:      emptyMapIfNil(record.Labels),
			Annotations: emptyMapIfNil(record.Annotations),
		},
	}
}

func ForServiceBindingList(serviceBindingRecords []repositories.ServiceBindingRecord, appRecords []repositories.AppRecord, baseURL, requestURL url.URL) ListResponse[ServiceBindingResponse] {
	includedApps := slices.Collect(it.Map(itx.FromSlice(appRecords), func(app repositories.AppRecord) include.Resource {
		return include.Resource{
			Type:     "apps",
			Resource: ForApp(app, baseURL),
		}
	}))

	return ForListDeprecated(ForServiceBinding, serviceBindingRecords, baseURL, requestURL, includedApps...)
}

func ForServiceBindingDetails(serviceBindingDetailsRecord repositories.ServiceBindingDetailsRecord) ServiceBindingDetailsResponse {
	return ServiceBindingDetailsResponse{
		Credentials: serviceBindingDetailsRecord.Credentials,
	}
}
