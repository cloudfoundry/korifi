package repositories

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const SecurityGroupResourceType = "Security Group"

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
	Rules           []korifiv1alpha1.SecurityGroupRule
	Spaces          map[string]korifiv1alpha1.SecurityGroupWorkloads
	GloballyEnabled korifiv1alpha1.SecurityGroupWorkloads
}

type SecurityGroupRecord struct {
	GUID            string
	CreatedAt       time.Time
	UpdatedAt       *time.Time
	DeletedAt       *time.Time
	Name            string
	Rules           []korifiv1alpha1.SecurityGroupRule
	GloballyEnabled korifiv1alpha1.SecurityGroupWorkloads
	RunningSpaces   []string
	StagingSpaces   []string
}

func (r *SecurityGroupRepo) CreateSecurityGroup(ctx context.Context, authInfo authorization.Info, message CreateSecurityGroupMessage) (SecurityGroupRecord, error) {
	cfSecurityGroup := &korifiv1alpha1.CFSecurityGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      uuid.NewString(),
		},
		Spec: korifiv1alpha1.CFSecurityGroupSpec{
			DisplayName:     message.DisplayName,
			Rules:           message.Rules,
			Spaces:          message.Spaces,
			GloballyEnabled: message.GloballyEnabled,
		},
	}

	if err := r.klient.Create(ctx, cfSecurityGroup); err != nil {
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
		GUID:            cfSecurityGroup.Name,
		CreatedAt:       cfSecurityGroup.CreationTimestamp.Time,
		DeletedAt:       golangTime(cfSecurityGroup.DeletionTimestamp),
		Name:            cfSecurityGroup.Spec.DisplayName,
		GloballyEnabled: cfSecurityGroup.Spec.GloballyEnabled,
		Rules:           cfSecurityGroup.Spec.Rules,
		UpdatedAt:       getLastUpdatedTime(&cfSecurityGroup),
		RunningSpaces:   runningSpaces,
		StagingSpaces:   stagingSpaces,
	}
}
