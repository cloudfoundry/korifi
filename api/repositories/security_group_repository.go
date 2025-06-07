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

const (
	SecurityGroupResourceType     = "Security Group"
	SecurityGroupRunningSpaceType = "running"
	SecurityGroupStagingSpaceType = "staging"
)

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
	klient        Klient
	rootNamespace string
}

func NewSecurityGroupRepo(
	klient Klient,
	rootNamespace string,
) *SecurityGroupRepo {
	return &SecurityGroupRepo{
		klient:        klient,
		rootNamespace: rootNamespace,
	}
}

type CreateSecurityGroupMessage struct {
	DisplayName     string
	Rules           []SecurityGroupRule
	Spaces          map[string]SecurityGroupWorkloads
	GloballyEnabled SecurityGroupWorkloads
}

type BindSecurityGroupMessage struct {
	GUID     string
	Spaces   []string
	Workload string
}

func (m BindSecurityGroupMessage) apply(cfSecurityGroup *korifiv1alpha1.CFSecurityGroup) {
	if cfSecurityGroup.Spec.Spaces == nil {
		cfSecurityGroup.Spec.Spaces = make(map[string]korifiv1alpha1.SecurityGroupWorkloads)
	}

	for _, space := range m.Spaces {
		workloads := cfSecurityGroup.Spec.Spaces[space]

		if m.Workload == SecurityGroupRunningSpaceType {
			workloads.Running = true
		} else {
			workloads.Staging = true
		}

		cfSecurityGroup.Spec.Spaces[space] = workloads
	}
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

func (r *SecurityGroupRepo) GetSecurityGroup(ctx context.Context, authInfo authorization.Info, GUID string) (SecurityGroupRecord, error) {
	cfSecurityGroup := &korifiv1alpha1.CFSecurityGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      GUID,
		},
	}

	if err := r.klient.Get(ctx, cfSecurityGroup); err != nil {
		return SecurityGroupRecord{}, fmt.Errorf("failed to get security group: %w", apierrors.FromK8sError(err, SecurityGroupResourceType))
	}

	return toSecurityGroupRecord(*cfSecurityGroup), nil
}

func (r *SecurityGroupRepo) CreateSecurityGroup(ctx context.Context, authInfo authorization.Info, message CreateSecurityGroupMessage) (SecurityGroupRecord, error) {
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

	if err := r.klient.Create(ctx, cfSecurityGroup); err != nil {
		if validationError, ok := validation.WebhookErrorToValidationError(err); ok {
			if validationError.Type == validation.DuplicateNameErrorType {
				return SecurityGroupRecord{}, apierrors.NewUniquenessError(err, validationError.GetMessage())
			}
		}

		return SecurityGroupRecord{}, apierrors.FromK8sError(err, SecurityGroupResourceType)
	}

	return toSecurityGroupRecord(*cfSecurityGroup), nil
}

func (r *SecurityGroupRepo) BindSecurityGroup(ctx context.Context, authInfo authorization.Info, message BindSecurityGroupMessage) (SecurityGroupRecord, error) {
	cfSecurityGroup := &korifiv1alpha1.CFSecurityGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      message.GUID,
		},
	}

	if err := r.klient.Get(ctx, cfSecurityGroup); err != nil {
		return SecurityGroupRecord{}, apierrors.FromK8sError(err, SecurityGroupResourceType)
	}

	if err := r.klient.Patch(ctx, cfSecurityGroup, func() error {
		message.apply(cfSecurityGroup)
		return nil
	}); err != nil {
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
