package descriptors_test

import (
	"fmt"
	"maps"
	"slices"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
)

var _ = DescribeTable("Table Columns For Ordering", testTableColumns,
	Entry("CFApps", "cfapps", &korifiv1alpha1.CFApp{
		Spec: korifiv1alpha1.CFAppSpec{
			DisplayName:  "cfapp",
			DesiredState: "STOPPED",
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
			},
		},
	}, MatchKeys(IgnoreExtras, Keys{
		"Created At":   Equal("obj-creation-timestamp"),
		"Updated At":   Equal("obj-update-timestamp"),
		"Display Name": Equal("cfapp"),
		"State":        Equal("STOPPED"),
	})),

	Entry("CFBuild", "cfbuilds", &korifiv1alpha1.CFBuild{
		Spec: korifiv1alpha1.CFBuildSpec{
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
			},
		},
	}, MatchKeys(IgnoreExtras, Keys{
		"Created At": Equal("obj-creation-timestamp"),
		"Updated At": Equal("obj-update-timestamp"),
	})),

	Entry("CFDomain", "cfdomains", &korifiv1alpha1.CFDomain{
		Spec: korifiv1alpha1.CFDomainSpec{
			Name: "example.com",
		},
	}, MatchKeys(IgnoreExtras, Keys{
		"Created At": Equal("obj-creation-timestamp"),
		"Updated At": Equal("obj-update-timestamp"),
	})),

	Entry("CFOrg", "cforgs", &korifiv1alpha1.CFOrg{
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: "example-org",
		},
	}, MatchKeys(IgnoreExtras, Keys{
		"Created At":   Equal("obj-creation-timestamp"),
		"Updated At":   Equal("obj-update-timestamp"),
		"Display Name": Equal("example-org"),
	})),

	Entry("CFPackage", "cfpackages", &korifiv1alpha1.CFPackage{
		Spec: korifiv1alpha1.CFPackageSpec{
			Type: "bits",
		},
	}, MatchKeys(IgnoreExtras, Keys{
		"Created At": Equal("obj-creation-timestamp"),
		"Updated At": Equal("obj-update-timestamp"),
	})),

	Entry("CFProcess", "cfprocesses", &korifiv1alpha1.CFProcess{}, MatchKeys(IgnoreExtras, Keys{
		"Created At": Equal("obj-creation-timestamp"),
		"Updated At": Equal("obj-update-timestamp"),
	})),

	Entry("CFRoute", "cfroutes", &korifiv1alpha1.CFRoute{
		Spec: korifiv1alpha1.CFRouteSpec{
			Host: "example",
			Path: "/example",
			DomainRef: corev1.ObjectReference{
				Name: "example.com",
			},
		},
	}, MatchKeys(IgnoreExtras, Keys{
		"Created At": Equal("obj-creation-timestamp"),
		"Updated At": Equal("obj-update-timestamp"),
	})),

	Entry("CFSecurityGroup", "cfsecuritygroups", &korifiv1alpha1.CFSecurityGroup{
		Spec: korifiv1alpha1.CFSecurityGroupSpec{
			Rules: []korifiv1alpha1.SecurityGroupRule{},
		},
	}, MatchKeys(IgnoreExtras, Keys{
		"Created At": Equal("obj-creation-timestamp"),
		"Updated At": Equal("obj-update-timestamp"),
	})),

	Entry("CFServiceBinding", "cfservicebindings", &korifiv1alpha1.CFServiceBinding{
		Spec: korifiv1alpha1.CFServiceBindingSpec{
			DisplayName: tools.PtrTo("example-binding"),
			Type:        "key",
		},
	}, MatchKeys(IgnoreExtras, Keys{
		"Created At":   Equal("obj-creation-timestamp"),
		"Updated At":   Equal("obj-update-timestamp"),
		"Display Name": Equal("example-binding"),
	})),

	Entry("CFServiceBroker", "cfservicebrokers", &korifiv1alpha1.CFServiceBroker{
		Spec: korifiv1alpha1.CFServiceBrokerSpec{
			Name: "example-broker",
		},
	}, MatchKeys(IgnoreExtras, Keys{
		"Created At":   Equal("obj-creation-timestamp"),
		"Updated At":   Equal("obj-update-timestamp"),
		"Display Name": Equal("example-broker"),
	})),

	Entry("CFServiceInstance", "cfserviceinstances", &korifiv1alpha1.CFServiceInstance{
		Spec: korifiv1alpha1.CFServiceInstanceSpec{
			DisplayName: "example-instance",
			Type:        "user-provided",
		},
	}, MatchKeys(IgnoreExtras, Keys{
		"Created At":   Equal("obj-creation-timestamp"),
		"Updated At":   Equal("obj-update-timestamp"),
		"Display Name": Equal("example-instance"),
	})),

	Entry("CFServiceOffering", "cfserviceofferings", &korifiv1alpha1.CFServiceOffering{
		Spec: korifiv1alpha1.CFServiceOfferingSpec{
			Name: "example-offering",
		},
	}, MatchKeys(IgnoreExtras, Keys{
		"Created At":   Equal("obj-creation-timestamp"),
		"Updated At":   Equal("obj-update-timestamp"),
		"Display Name": Equal("example-offering"),
	})),

	Entry("CFServicePlan", "cfserviceplans", &korifiv1alpha1.CFServicePlan{
		Spec: korifiv1alpha1.CFServicePlanSpec{
			Name: "example-plan",
			Visibility: korifiv1alpha1.ServicePlanVisibility{
				Type: korifiv1alpha1.PublicServicePlanVisibilityType,
			},
		},
	}, MatchKeys(IgnoreExtras, Keys{
		"Created At":   Equal("obj-creation-timestamp"),
		"Updated At":   Equal("obj-update-timestamp"),
		"Display Name": Equal("example-plan"),
	})),

	Entry("CFSpace", "cfspaces", &korifiv1alpha1.CFSpace{
		Spec: korifiv1alpha1.CFSpaceSpec{
			DisplayName: "example-space",
		},
	}, MatchKeys(IgnoreExtras, Keys{
		"Created At":   Equal("obj-creation-timestamp"),
		"Updated At":   Equal("obj-update-timestamp"),
		"Display Name": Equal("example-space"),
	})),

	Entry("CFTask", "cftasks", &korifiv1alpha1.CFTask{}, MatchKeys(IgnoreExtras, Keys{
		"Created At": Equal("obj-creation-timestamp"),
		"Updated At": Equal("obj-update-timestamp"),
	})),
)

func testTableColumns(resourceType string, obj client.Object, match types.GomegaMatcher) {
	namespace := uuid.NewString()
	Expect(k8sClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	})).To(Succeed())

	obj.SetName(uuid.NewString())
	obj.SetNamespace(namespace)
	obj.SetLabels(map[string]string{
		korifiv1alpha1.CreatedAtLabelKey: "obj-creation-timestamp",
		korifiv1alpha1.UpdatedAtLabelKey: "obj-update-timestamp",
	})
	Expect(k8sClient.Create(ctx, obj)).To(Succeed())

	table := &metav1.Table{}
	Expect(restClient.Get().
		AbsPath(fmt.Sprintf("/apis/korifi.cloudfoundry.org/v1alpha1/namespaces/%s/%s", namespace, resourceType)).
		SetHeader("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1").
		Do(ctx).
		Into(table),
	).To(Succeed())

	Expect(columnNameValues(table)).To(match)
}

func columnNameValues(table *metav1.Table) map[string]any {
	GinkgoHelper()

	columnsNames := it.Map(slices.Values(table.ColumnDefinitions), func(column metav1.TableColumnDefinition) string {
		return column.Name
	})
	Expect(table.Rows).To(HaveLen(1))
	firstRowCells := slices.Values(table.Rows[0].Cells)
	return maps.Collect(it.Zip(columnsNames, firstRowCells))
}
