package repositories

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

const (
	OrgNameLabel = "cloudfoundry.org/org-name"
)

type OrgRecord struct {
	Name      string
	GUID      string
	Suspended bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type OrgRepo struct {
	rootNamespace    string
	privilegedClient client.Client
}

func NewOrgRepo(rootNamespace string, privilegedClient client.Client) *OrgRepo {
	return &OrgRepo{
		rootNamespace:    rootNamespace,
		privilegedClient: privilegedClient,
	}
}

func (r *OrgRepo) FetchOrgs(ctx context.Context, names []string) ([]OrgRecord, error) {
	subnamespaceAnchorList := &v1alpha2.SubnamespaceAnchorList{}

	options := []client.ListOption{client.InNamespace(r.rootNamespace)}
	if len(names) > 0 {
		namesRequirement, err := labels.NewRequirement(OrgNameLabel, selection.In, names)
		if err != nil {
			return nil, err
		}
		namesSelector := client.MatchingLabelsSelector{
			Selector: labels.NewSelector().Add(*namesRequirement),
		}
		options = append(options, namesSelector)
	}

	err := r.privilegedClient.List(ctx, subnamespaceAnchorList, options...)
	if err != nil {
		return nil, err
	}

	records := []OrgRecord{}
	for _, anchor := range subnamespaceAnchorList.Items {
		records = append(records, OrgRecord{
			Name:      anchor.Labels[OrgNameLabel],
			GUID:      string(anchor.UID),
			CreatedAt: anchor.CreationTimestamp.Time,
			UpdatedAt: anchor.CreationTimestamp.Time,
		})
	}

	return records, nil
}
