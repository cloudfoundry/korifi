package payloads

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

type PackageCreate struct {
	Type          string                `json:"type" validate:"required,oneof='bits'"`
	Relationships *PackageRelationships `json:"relationships" validate:"required"`
}

type PackageRelationships struct {
	App *Relationship `json:"app" validate:"required"`
}

func (m PackageCreate) ToMessage(record repositories.AppRecord) repositories.CreatePackageMessage {
	return repositories.CreatePackageMessage{
		Type:      m.Type,
		AppGUID:   record.GUID,
		SpaceGUID: record.SpaceGUID,
		OwnerRef: metav1.OwnerReference{
			APIVersion: repositories.APIVersion,
			Kind:       "CFApp",
			Name:       record.GUID,
			UID:        record.EtcdUID,
		},
	}
}

type PackageListQueryParameters struct {
	AppGUIDs string `schema:"app_guids"`
	OrderBy  string `schema:"order_by"`
	States   string `schema:"states"`

	// Below parameters are ignored, but must be included to ignore as query parameters
	PerPage string `schema:"per_page"`
}

func (p *PackageListQueryParameters) ToMessage() repositories.ListPackagesMessage {
	var descendingOrder bool

	if strings.HasPrefix(p.OrderBy, "-") {
		descendingOrder = true
	}

	return repositories.ListPackagesMessage{
		AppGUIDs:        parseArrayParam(p.AppGUIDs),
		SortBy:          strings.TrimPrefix(p.OrderBy, "-"),
		DescendingOrder: descendingOrder,
		States:          parseArrayParam(p.States),
	}
}

func (p *PackageListQueryParameters) SupportedQueryParameters() []string {
	return []string{"app_guids", "order_by", "per_page", "states"}
}

type PackageListDropletsQueryParameters struct {
	// Below parameters are ignored, but must be included to ignore as query parameters
	States  string `schema:"states"`
	PerPage string `schema:"per_page"`
}

func (p *PackageListDropletsQueryParameters) ToMessage(packageGUIDs []string) repositories.ListDropletsMessage {
	return repositories.ListDropletsMessage{
		PackageGUIDs: packageGUIDs,
	}
}

func (p *PackageListDropletsQueryParameters) SupportedQueryParameters() []string {
	return []string{"states", "per_page"}
}
