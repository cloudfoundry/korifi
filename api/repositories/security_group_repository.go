package repositories

import (
	"context"
	"fmt"
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const SecurityGroupResourceType = "Security Group"

type SecurityGroupRule struct {
	Protocol    string `json:"protocol"`
	Destination string `json:"destination"`
	Ports       string `json:"ports,omitempty"`
	Type        int    `json:"type,omitempty"`
	Code        int    `json:"code,omitempty"`
	Description string `json:"description,omitempty"`
	Log         bool   `json:"log,omitempty"`
}

type SecurityGroupWorkloads struct {
	Running bool `json:"running"`
	Staging bool `json:"staging"`
}

type SecurityGroupRepo struct {
	userClientFactory authorization.UserClientFactory
	rootNamespace     string
}

func NewSecurityGroupRepo(
	userClientFactory authorization.UserClientFactory,
	rootNamespace string,
) *SecurityGroupRepo {
	return &SecurityGroupRepo{
		userClientFactory: userClientFactory,
		rootNamespace:     rootNamespace,
	}
}

type CreateSecurityGroupMessage struct {
	DisplayName     string
	Rules           []SecurityGroupRule
	Spaces          map[string]SecurityGroupWorkloads
	GloballyEnabled SecurityGroupWorkloads
}

type SecurityGroupRecord struct {
	GUID            string
	CreatedAt       time.Time
	UpdatedAt       *time.Time
	DeletedAt       *time.Time
	Name            string
	Rules           []SecurityGroupRule
	GloballyEnabled SecurityGroupWorkloads
	RunningSpaces   []string
	StagingSpaces   []string
}

func (r *SecurityGroupRepo) CreateSecurityGroup(ctx context.Context, authInfo authorization.Info, message CreateSecurityGroupMessage) (SecurityGroupRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return SecurityGroupRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfSecurityGroup := &korifiv1alpha1.CFSecurityGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      uuid.NewString(),
		},
		Spec: korifiv1alpha1.CFSecurityGroupSpec{
			DisplayName: message.DisplayName,
			Rules: slices.Collect(it.Map(slices.Values(message.Rules), func(r SecurityGroupRule) korifiv1alpha1.SecurityGroupRule {
				return korifiv1alpha1.SecurityGroupRule{
					Protocol:    r.Protocol,
					Destination: r.Destination,
					Ports:       r.Ports,
					Type:        r.Type,
					Code:        r.Code,
					Description: r.Description,
					Log:         r.Log,
				}
			})),
			Spaces: func() map[string]korifiv1alpha1.SecurityGroupWorkloads {
				spaces := make(map[string]korifiv1alpha1.SecurityGroupWorkloads, len(message.Spaces))
				for guid, workloads := range message.Spaces {
					spaces[guid] = korifiv1alpha1.SecurityGroupWorkloads{
						Running: workloads.Running,
						Staging: workloads.Staging,
					}
				}
				return spaces
			}(),
			GloballyEnabled: korifiv1alpha1.SecurityGroupWorkloads{
				Running: message.GloballyEnabled.Running,
				Staging: message.GloballyEnabled.Staging,
			},
		},
	}

	if err = userClient.Create(ctx, cfSecurityGroup); err != nil {
		if validationError, ok := validation.WebhookErrorToValidationError(err); ok {
			if validationError.Type == validation.DuplicateNameErrorType {
				return SecurityGroupRecord{}, apierrors.NewUniquenessError(err, validationError.GetMessage())
			}
		}

		return SecurityGroupRecord{}, apierrors.FromK8sError(err, SecurityGroupResourceType)
	}

	return toSecurityGroupRecord(*cfSecurityGroup), nil
}

func toSecurityGroupRecord(cfSecurityGroup korifiv1alpha1.CFSecurityGroup) SecurityGroupRecord {
	runningSpaces := []string{}
	stagingSpaces := []string{}

	for space, workloads := range cfSecurityGroup.Spec.Spaces {
		if workloads.Running {
			runningSpaces = append(runningSpaces, space)
		}
		if workloads.Staging {
			stagingSpaces = append(stagingSpaces, space)
		}
	}

	return SecurityGroupRecord{
		GUID:      cfSecurityGroup.Name,
		CreatedAt: cfSecurityGroup.CreationTimestamp.Time,
		DeletedAt: golangTime(cfSecurityGroup.DeletionTimestamp),
		Name:      cfSecurityGroup.Spec.DisplayName,
		GloballyEnabled: SecurityGroupWorkloads{
			Running: cfSecurityGroup.Spec.GloballyEnabled.Running,
			Staging: cfSecurityGroup.Spec.GloballyEnabled.Staging,
		},
		Rules: slices.Collect(it.Map(slices.Values(cfSecurityGroup.Spec.Rules), func(r korifiv1alpha1.SecurityGroupRule) SecurityGroupRule {
			return SecurityGroupRule{
				Protocol:    r.Protocol,
				Destination: r.Destination,
				Ports:       r.Ports,
				Type:        r.Type,
				Code:        r.Code,
				Description: r.Description,
				Log:         r.Log,
			}
		})),
		UpdatedAt:     getLastUpdatedTime(&cfSecurityGroup),
		RunningSpaces: runningSpaces,
		StagingSpaces: stagingSpaces,
	}
}
