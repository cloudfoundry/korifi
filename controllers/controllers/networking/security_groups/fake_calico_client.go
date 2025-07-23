package securitygroups

import (
	"context"
	"fmt"
	"strings"

	v3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	in "github.com/projectcalico/api/pkg/client/clientset_generated/clientset/typed/projectcalico/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
)

type FakeCalicoClient struct {
	networkPolicies       map[string]*v3.NetworkPolicy
	globalNetworkPolicies map[string]*v3.GlobalNetworkPolicy
}

func NewFakeCalicoClient() *FakeCalicoClient {
	return &FakeCalicoClient{
		networkPolicies:       make(map[string]*v3.NetworkPolicy),
		globalNetworkPolicies: make(map[string]*v3.GlobalNetworkPolicy),
	}
}

func (f *FakeCalicoClient) ProjectcalicoV3() in.ProjectcalicoV3Interface {
	return &fakeProjectcalicoV3Client{f}
}

func (f *FakeCalicoClient) Discovery() discovery.DiscoveryInterface {
	return nil
}

type fakeProjectcalicoV3Client struct {
	f *FakeCalicoClient
}

func (f *fakeProjectcalicoV3Client) BGPConfigurations() in.BGPConfigurationInterface {
	return &fakeBGPConfigurationClient{}
}

func (f *fakeProjectcalicoV3Client) BGPFilters() in.BGPFilterInterface {
	return &fakeBGFilterClient{}
}

func (f *fakeProjectcalicoV3Client) BGPPeers() in.BGPPeerInterface {
	return &fakeBGPPeerClient{}
}

func (f *fakeProjectcalicoV3Client) BlockAffinities() in.BlockAffinityInterface {
	return &fakeBlockAffinityClient{}
}

func (f *fakeProjectcalicoV3Client) CalicoNodeStatuses() in.CalicoNodeStatusInterface {
	return &fakeCalicoNodeStatusClient{}
}

func (f *fakeProjectcalicoV3Client) ClusterInformations() in.ClusterInformationInterface {
	return &fakeClusterInformationClient{}
}

func (f *fakeProjectcalicoV3Client) FelixConfigurations() in.FelixConfigurationInterface {
	return &fakeFelixConfigurationClient{}
}

func (f *fakeProjectcalicoV3Client) GlobalNetworkPolicies() in.GlobalNetworkPolicyInterface {
	return &fakeGlobalNetworkPolicyClient{f.f}
}

func (f *fakeProjectcalicoV3Client) GlobalNetworkSets() in.GlobalNetworkSetInterface {
	return &fakeGlobalNetworkSetClient{}
}

func (f *fakeProjectcalicoV3Client) HostEndpoints() in.HostEndpointInterface {
	return &fakeHostEndpointClient{}
}

func (f *fakeProjectcalicoV3Client) IPAMConfigurations() in.IPAMConfigurationInterface {
	return &fakeIPAMConfigurationClient{}
}

func (f *fakeProjectcalicoV3Client) IPPools() in.IPPoolInterface {
	return &fakeIPPoolClient{}
}

func (f *fakeProjectcalicoV3Client) IPReservations() in.IPReservationInterface {
	return &fakeIPReservationClient{}
}

func (f *fakeProjectcalicoV3Client) KubeControllersConfigurations() in.KubeControllersConfigurationInterface {
	return &fakeKubeControllersConfigurationClient{}
}

func (f *fakeProjectcalicoV3Client) NetworkPolicies(namespace string) in.NetworkPolicyInterface {
	return &fakeNetworkPolicyClient{f.f, namespace}
}

func (f *fakeProjectcalicoV3Client) NetworkSets(namespace string) in.NetworkSetInterface {
	return &fakeNetworkSetClient{}
}

func (f *fakeProjectcalicoV3Client) Profiles() in.ProfileInterface {
	return &fakeProfileClient{}
}

func (f *fakeProjectcalicoV3Client) StagedGlobalNetworkPolicies() in.StagedGlobalNetworkPolicyInterface {
	return &fakeStagedGlobalNetworkPolicyClient{}
}

func (f *fakeProjectcalicoV3Client) StagedKubernetesNetworkPolicies(s string) in.StagedKubernetesNetworkPolicyInterface {
	return &fakeStagedKubernetesNetworkPolicyClient{}
}

func (f *fakeProjectcalicoV3Client) StagedNetworkPolicies(s string) in.StagedNetworkPolicyInterface {
	return &fakeStagedNetworkPolicyClient{}
}

func (f *fakeProjectcalicoV3Client) Tiers() in.TierInterface {
	return &fakeTierClient{}
}

func (c *fakeProjectcalicoV3Client) RESTClient() rest.Interface {
	return &fakeRESTInterfaceClient{}
}

type fakeNetworkPolicyClient struct {
	f         *FakeCalicoClient
	namespace string
}

func (f *fakeNetworkPolicyClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.NetworkPolicy, error) {
	key := fmt.Sprintf("%s/%s", f.namespace, name)
	if policy, exists := f.f.networkPolicies[key]; exists {
		return policy, nil
	}
	return nil, apierrors.NewNotFound(v3.Resource("NetworkPolicy"), name)
}

func (f *fakeNetworkPolicyClient) Create(ctx context.Context, policy *v3.NetworkPolicy, opts metav1.CreateOptions) (*v3.NetworkPolicy, error) {
	key := fmt.Sprintf("%s/default.%s", policy.Namespace, policy.Name)
	f.f.networkPolicies[key] = policy
	return policy, nil
}

func (f *fakeNetworkPolicyClient) Update(ctx context.Context, policy *v3.NetworkPolicy, opts metav1.UpdateOptions) (*v3.NetworkPolicy, error) {
	key := fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)
	f.f.networkPolicies[key] = policy
	return policy, nil
}

func (f *fakeNetworkPolicyClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	key := fmt.Sprintf("%s/%s", f.namespace, name)
	if _, exists := f.f.networkPolicies[key]; exists {
		delete(f.f.networkPolicies, key)
		return nil
	}
	return apierrors.NewNotFound(v3.Resource("NetworkPolicy"), name)
}

func (f *fakeNetworkPolicyClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.NetworkPolicyList, error) {
	result := &v3.NetworkPolicyList{Items: []v3.NetworkPolicy{}}
	for key, policy := range f.f.networkPolicies {
		if f.namespace != "" && !strings.HasPrefix(key, f.namespace+"/") {
			continue
		}

		if opts.FieldSelector != "" {
			if !strings.Contains(opts.FieldSelector, "metadata.name=") {
				continue
			}
			name := strings.Split(strings.Split(opts.FieldSelector, "=")[1], ".")[1]
			if policy.Name != fmt.Sprintf("default.%s", name) {
				continue
			}
		}

		if opts.LabelSelector != "" && !matchesLabelSelector(policy.Labels, opts.LabelSelector) {
			continue
		}
		result.Items = append(result.Items, *policy)
	}
	return result, nil
}

func (f *fakeNetworkPolicyClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeNetworkPolicyClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeNetworkPolicyClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.NetworkPolicy, err error) {
	return nil, nil
}

type fakeGlobalNetworkPolicyClient struct {
	f *FakeCalicoClient
}

func (f *fakeGlobalNetworkPolicyClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.GlobalNetworkPolicy, error) {
	if policy, exists := f.f.globalNetworkPolicies[name]; exists {
		return policy, nil
	}
	return nil, apierrors.NewNotFound(v3.Resource("GlobalNetworkPolicy"), name)
}

func (f *fakeGlobalNetworkPolicyClient) Create(ctx context.Context, policy *v3.GlobalNetworkPolicy, opts metav1.CreateOptions) (*v3.GlobalNetworkPolicy, error) {
	key := fmt.Sprintf("default.%s", policy.Name)
	f.f.globalNetworkPolicies[key] = policy
	return policy, nil
}

func (f *fakeGlobalNetworkPolicyClient) Update(ctx context.Context, policy *v3.GlobalNetworkPolicy, opts metav1.UpdateOptions) (*v3.GlobalNetworkPolicy, error) {
	f.f.globalNetworkPolicies[policy.Name] = policy
	return policy, nil
}

func (f *fakeGlobalNetworkPolicyClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	if _, exists := f.f.globalNetworkPolicies[name]; exists {
		delete(f.f.globalNetworkPolicies, name)
		return nil
	}
	return apierrors.NewNotFound(v3.Resource("GlobalNetworkPolicy"), name)
}

func (f *fakeGlobalNetworkPolicyClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.GlobalNetworkPolicyList, error) {
	result := &v3.GlobalNetworkPolicyList{Items: []v3.GlobalNetworkPolicy{}}
	for _, policy := range f.f.globalNetworkPolicies {
		if opts.FieldSelector != "" {
			if !strings.Contains(opts.FieldSelector, "metadata.name=") {
				continue
			}
			name := strings.Split(strings.Split(opts.FieldSelector, "=")[1], ".")[1]
			if policy.Name != fmt.Sprintf("default.%s", name) {
				continue
			}
		}

		if opts.LabelSelector != "" && !matchesLabelSelector(policy.Labels, opts.LabelSelector) {
			continue
		}
		result.Items = append(result.Items, *policy)
	}
	return result, nil
}

func (f *fakeGlobalNetworkPolicyClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeGlobalNetworkPolicyClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeGlobalNetworkPolicyClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.GlobalNetworkPolicy, err error) {
	return nil, nil
}

func matchesLabelSelector(labels map[string]string, selector string) bool {
	parts := strings.Split(selector, "=")
	if len(parts) != 2 {
		return false
	}
	key, value := parts[0], parts[1]
	return labels[key] == value
}

type fakeBGPConfigurationClient struct{}

func (f *fakeBGPConfigurationClient) Create(ctx context.Context, bGPConfiguration *v3.BGPConfiguration, opts metav1.CreateOptions) (*v3.BGPConfiguration, error) {
	return nil, nil
}

func (f *fakeBGPConfigurationClient) Update(ctx context.Context, bGPConfiguration *v3.BGPConfiguration, opts metav1.UpdateOptions) (*v3.BGPConfiguration, error) {
	return nil, nil
}

func (f *fakeBGPConfigurationClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeBGPConfigurationClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeBGPConfigurationClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.BGPConfiguration, error) {
	return nil, nil
}

func (f *fakeBGPConfigurationClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.BGPConfigurationList, error) {
	return nil, nil
}

func (f *fakeBGPConfigurationClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeBGPConfigurationClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*v3.BGPConfiguration, error) {
	return nil, nil
}

type fakeBGFilterClient struct{}

func (f *fakeBGFilterClient) Create(ctx context.Context, bGPFilter *v3.BGPFilter, opts metav1.CreateOptions) (*v3.BGPFilter, error) {
	return nil, nil
}

func (f *fakeBGFilterClient) Update(ctx context.Context, bGPFilter *v3.BGPFilter, opts metav1.UpdateOptions) (*v3.BGPFilter, error) {
	return nil, nil
}

func (f *fakeBGFilterClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeBGFilterClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeBGFilterClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.BGPFilter, error) {
	return nil, nil
}

func (f *fakeBGFilterClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.BGPFilterList, error) {
	return nil, nil
}

func (f *fakeBGFilterClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeBGFilterClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.BGPFilter, err error) {
	return nil, nil
}

type fakeBGPPeerClient struct{}

func (f *fakeBGPPeerClient) Create(ctx context.Context, bGPPeer *v3.BGPPeer, opts metav1.CreateOptions) (*v3.BGPPeer, error) {
	return nil, nil
}

func (f *fakeBGPPeerClient) Update(ctx context.Context, bGPPeer *v3.BGPPeer, opts metav1.UpdateOptions) (*v3.BGPPeer, error) {
	return nil, nil
}

func (f *fakeBGPPeerClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeBGPPeerClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeBGPPeerClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.BGPPeer, error) {
	return nil, nil
}

func (f *fakeBGPPeerClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.BGPPeerList, error) {
	return nil, nil
}

func (f *fakeBGPPeerClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeBGPPeerClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.BGPPeer, err error) {
	return nil, nil
}

type fakeBlockAffinityClient struct{}

func (f *fakeBlockAffinityClient) Create(ctx context.Context, blockAffinity *v3.BlockAffinity, opts metav1.CreateOptions) (*v3.BlockAffinity, error) {
	return nil, nil
}

func (f *fakeBlockAffinityClient) Update(ctx context.Context, blockAffinity *v3.BlockAffinity, opts metav1.UpdateOptions) (*v3.BlockAffinity, error) {
	return nil, nil
}

func (f *fakeBlockAffinityClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeBlockAffinityClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeBlockAffinityClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.BlockAffinity, error) {
	return nil, nil
}

func (f *fakeBlockAffinityClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.BlockAffinityList, error) {
	return nil, nil
}

func (f *fakeBlockAffinityClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeBlockAffinityClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.BlockAffinity, err error) {
	return nil, nil
}

type fakeCalicoNodeStatusClient struct{}

func (f *fakeCalicoNodeStatusClient) Create(ctx context.Context, calicoNodeStatus *v3.CalicoNodeStatus, opts metav1.CreateOptions) (*v3.CalicoNodeStatus, error) {
	return nil, nil
}

func (f *fakeCalicoNodeStatusClient) Update(ctx context.Context, calicoNodeStatus *v3.CalicoNodeStatus, opts metav1.UpdateOptions) (*v3.CalicoNodeStatus, error) {
	return nil, nil
}

func (f *fakeCalicoNodeStatusClient) UpdateStatus(ctx context.Context, calicoNodeStatus *v3.CalicoNodeStatus, opts metav1.UpdateOptions) (*v3.CalicoNodeStatus, error) {
	return nil, nil
}

func (f *fakeCalicoNodeStatusClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeCalicoNodeStatusClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeCalicoNodeStatusClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.CalicoNodeStatus, error) {
	return nil, nil
}

func (f *fakeCalicoNodeStatusClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.CalicoNodeStatusList, error) {
	return nil, nil
}

func (f *fakeCalicoNodeStatusClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeCalicoNodeStatusClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.CalicoNodeStatus, err error) {
	return nil, nil
}

type fakeClusterInformationClient struct{}

func (f *fakeClusterInformationClient) Create(ctx context.Context, clusterInformation *v3.ClusterInformation, opts metav1.CreateOptions) (*v3.ClusterInformation, error) {
	return nil, nil
}

func (f *fakeClusterInformationClient) Update(ctx context.Context, clusterInformation *v3.ClusterInformation, opts metav1.UpdateOptions) (*v3.ClusterInformation, error) {
	return nil, nil
}

func (f *fakeClusterInformationClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeClusterInformationClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeClusterInformationClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.ClusterInformation, error) {
	return nil, nil
}

func (f *fakeClusterInformationClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.ClusterInformationList, error) {
	return nil, nil
}

func (f *fakeClusterInformationClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeClusterInformationClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.ClusterInformation, err error) {
	return nil, nil
}

type fakeFelixConfigurationClient struct{}

func (f *fakeFelixConfigurationClient) Create(ctx context.Context, felixConfiguration *v3.FelixConfiguration, opts metav1.CreateOptions) (*v3.FelixConfiguration, error) {
	return nil, nil
}

func (f *fakeFelixConfigurationClient) Update(ctx context.Context, felixConfiguration *v3.FelixConfiguration, opts metav1.UpdateOptions) (*v3.FelixConfiguration, error) {
	return nil, nil
}

func (f *fakeFelixConfigurationClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeFelixConfigurationClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeFelixConfigurationClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.FelixConfiguration, error) {
	return nil, nil
}

func (f *fakeFelixConfigurationClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.FelixConfigurationList, error) {
	return nil, nil
}

func (f *fakeFelixConfigurationClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeFelixConfigurationClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.FelixConfiguration, err error) {
	return nil, nil
}

type fakeGlobalNetworkSetClient struct{}

func (f *fakeGlobalNetworkSetClient) Create(ctx context.Context, globalNetworkSet *v3.GlobalNetworkSet, opts metav1.CreateOptions) (*v3.GlobalNetworkSet, error) {
	return nil, nil
}

func (f *fakeGlobalNetworkSetClient) Update(ctx context.Context, globalNetworkSet *v3.GlobalNetworkSet, opts metav1.UpdateOptions) (*v3.GlobalNetworkSet, error) {
	return nil, nil
}

func (f *fakeGlobalNetworkSetClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeGlobalNetworkSetClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeGlobalNetworkSetClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.GlobalNetworkSet, error) {
	return nil, nil
}

func (f *fakeGlobalNetworkSetClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.GlobalNetworkSetList, error) {
	return nil, nil
}

func (f *fakeGlobalNetworkSetClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeGlobalNetworkSetClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.GlobalNetworkSet, err error) {
	return nil, nil
}

type fakeHostEndpointClient struct{}

func (f *fakeHostEndpointClient) Create(ctx context.Context, hostEndpoint *v3.HostEndpoint, opts metav1.CreateOptions) (*v3.HostEndpoint, error) {
	return nil, nil
}

func (f *fakeHostEndpointClient) Update(ctx context.Context, hostEndpoint *v3.HostEndpoint, opts metav1.UpdateOptions) (*v3.HostEndpoint, error) {
	return nil, nil
}

func (f *fakeHostEndpointClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeHostEndpointClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeHostEndpointClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.HostEndpoint, error) {
	return nil, nil
}

func (f *fakeHostEndpointClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.HostEndpointList, error) {
	return nil, nil
}

func (f *fakeHostEndpointClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeHostEndpointClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.HostEndpoint, err error) {
	return nil, nil
}

type fakeIPAMConfigurationClient struct{}

func (f *fakeIPAMConfigurationClient) Create(ctx context.Context, iPAMConfiguration *v3.IPAMConfiguration, opts metav1.CreateOptions) (*v3.IPAMConfiguration, error) {
	return nil, nil
}

func (f *fakeIPAMConfigurationClient) Update(ctx context.Context, iPAMConfiguration *v3.IPAMConfiguration, opts metav1.UpdateOptions) (*v3.IPAMConfiguration, error) {
	return nil, nil
}

func (f *fakeIPAMConfigurationClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeIPAMConfigurationClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeIPAMConfigurationClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.IPAMConfiguration, error) {
	return nil, nil
}

func (f *fakeIPAMConfigurationClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.IPAMConfigurationList, error) {
	return nil, nil
}

func (f *fakeIPAMConfigurationClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeIPAMConfigurationClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.IPAMConfiguration, err error) {
	return nil, nil
}

type fakeIPPoolClient struct{}

func (f *fakeIPPoolClient) Create(ctx context.Context, iPPool *v3.IPPool, opts metav1.CreateOptions) (*v3.IPPool, error) {
	return nil, nil
}

func (f *fakeIPPoolClient) Update(ctx context.Context, iPPool *v3.IPPool, opts metav1.UpdateOptions) (*v3.IPPool, error) {
	return nil, nil
}

func (f *fakeIPPoolClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeIPPoolClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeIPPoolClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.IPPool, error) {
	return nil, nil
}

func (f *fakeIPPoolClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.IPPoolList, error) {
	return nil, nil
}

func (f *fakeIPPoolClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeIPPoolClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.IPPool, err error) {
	return nil, nil
}

type fakeIPReservationClient struct{}

func (f *fakeIPReservationClient) Create(ctx context.Context, iPReservation *v3.IPReservation, opts metav1.CreateOptions) (*v3.IPReservation, error) {
	return nil, nil
}

func (f *fakeIPReservationClient) Update(ctx context.Context, iPReservation *v3.IPReservation, opts metav1.UpdateOptions) (*v3.IPReservation, error) {
	return nil, nil
}

func (f *fakeIPReservationClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeIPReservationClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeIPReservationClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.IPReservation, error) {
	return nil, nil
}

func (f *fakeIPReservationClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.IPReservationList, error) {
	return nil, nil
}

func (f *fakeIPReservationClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeIPReservationClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.IPReservation, err error) {
	return nil, nil
}

type fakeKubeControllersConfigurationClient struct{}

func (f *fakeKubeControllersConfigurationClient) Create(ctx context.Context, kubeControllersConfiguration *v3.KubeControllersConfiguration, opts metav1.CreateOptions) (*v3.KubeControllersConfiguration, error) {
	return nil, nil
}

func (f *fakeKubeControllersConfigurationClient) Update(ctx context.Context, kubeControllersConfiguration *v3.KubeControllersConfiguration, opts metav1.UpdateOptions) (*v3.KubeControllersConfiguration, error) {
	return nil, nil
}

func (f *fakeKubeControllersConfigurationClient) UpdateStatus(ctx context.Context, kubeControllersConfiguration *v3.KubeControllersConfiguration, opts metav1.UpdateOptions) (*v3.KubeControllersConfiguration, error) {
	return nil, nil
}

func (f *fakeKubeControllersConfigurationClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeKubeControllersConfigurationClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeKubeControllersConfigurationClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.KubeControllersConfiguration, error) {
	return nil, nil
}

func (f *fakeKubeControllersConfigurationClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.KubeControllersConfigurationList, error) {
	return nil, nil
}

func (f *fakeKubeControllersConfigurationClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeKubeControllersConfigurationClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.KubeControllersConfiguration, err error) {
	return nil, nil
}

type fakeNetworkSetClient struct{}

func (f *fakeNetworkSetClient) Create(ctx context.Context, networkSet *v3.NetworkSet, opts metav1.CreateOptions) (*v3.NetworkSet, error) {
	return nil, nil
}

func (f *fakeNetworkSetClient) Update(ctx context.Context, networkSet *v3.NetworkSet, opts metav1.UpdateOptions) (*v3.NetworkSet, error) {
	return nil, nil
}

func (f *fakeNetworkSetClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeNetworkSetClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeNetworkSetClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.NetworkSet, error) {
	return nil, nil
}

func (f *fakeNetworkSetClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.NetworkSetList, error) {
	return nil, nil
}

func (f *fakeNetworkSetClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeNetworkSetClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.NetworkSet, err error) {
	return nil, nil
}

type fakeProfileClient struct{}

func (f *fakeProfileClient) Create(ctx context.Context, profile *v3.Profile, opts metav1.CreateOptions) (*v3.Profile, error) {
	return nil, nil
}

func (f *fakeProfileClient) Update(ctx context.Context, profile *v3.Profile, opts metav1.UpdateOptions) (*v3.Profile, error) {
	return nil, nil
}

func (f *fakeProfileClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeProfileClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeProfileClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.Profile, error) {
	return nil, nil
}

func (f *fakeProfileClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.ProfileList, error) {
	return nil, nil
}

func (f *fakeProfileClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeProfileClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.Profile, err error) {
	return nil, nil
}

type fakeStagedGlobalNetworkPolicyClient struct{}

func (f *fakeStagedGlobalNetworkPolicyClient) Create(ctx context.Context, stagedGlobalNetworkPolicy *v3.StagedGlobalNetworkPolicy, opts metav1.CreateOptions) (*v3.StagedGlobalNetworkPolicy, error) {
	return nil, nil
}

func (f *fakeStagedGlobalNetworkPolicyClient) Update(ctx context.Context, stagedGlobalNetworkPolicy *v3.StagedGlobalNetworkPolicy, opts metav1.UpdateOptions) (*v3.StagedGlobalNetworkPolicy, error) {
	return nil, nil
}

func (f *fakeStagedGlobalNetworkPolicyClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeStagedGlobalNetworkPolicyClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeStagedGlobalNetworkPolicyClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.StagedGlobalNetworkPolicy, error) {
	return nil, nil
}

func (f *fakeStagedGlobalNetworkPolicyClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.StagedGlobalNetworkPolicyList, error) {
	return nil, nil
}

func (f *fakeStagedGlobalNetworkPolicyClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeStagedGlobalNetworkPolicyClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.StagedGlobalNetworkPolicy, err error) {
	return nil, nil
}

type fakeStagedKubernetesNetworkPolicyClient struct{}

func (f *fakeStagedKubernetesNetworkPolicyClient) Create(ctx context.Context, stagedKubernetesNetworkPolicy *v3.StagedKubernetesNetworkPolicy, opts metav1.CreateOptions) (*v3.StagedKubernetesNetworkPolicy, error) {
	return nil, nil
}

func (f *fakeStagedKubernetesNetworkPolicyClient) Update(ctx context.Context, stagedKubernetesNetworkPolicy *v3.StagedKubernetesNetworkPolicy, opts metav1.UpdateOptions) (*v3.StagedKubernetesNetworkPolicy, error) {
	return nil, nil
}

func (f *fakeStagedKubernetesNetworkPolicyClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeStagedKubernetesNetworkPolicyClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeStagedKubernetesNetworkPolicyClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.StagedKubernetesNetworkPolicy, error) {
	return nil, nil
}

func (f *fakeStagedKubernetesNetworkPolicyClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.StagedKubernetesNetworkPolicyList, error) {
	return nil, nil
}

func (f *fakeStagedKubernetesNetworkPolicyClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeStagedKubernetesNetworkPolicyClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.StagedKubernetesNetworkPolicy, err error) {
	return nil, nil
}

type fakeStagedNetworkPolicyClient struct{}

func (f *fakeStagedNetworkPolicyClient) Create(ctx context.Context, stagedNetworkPolicy *v3.StagedNetworkPolicy, opts metav1.CreateOptions) (*v3.StagedNetworkPolicy, error) {
	return nil, nil
}

func (f *fakeStagedNetworkPolicyClient) Update(ctx context.Context, stagedNetworkPolicy *v3.StagedNetworkPolicy, opts metav1.UpdateOptions) (*v3.StagedNetworkPolicy, error) {
	return nil, nil
}

func (f *fakeStagedNetworkPolicyClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeStagedNetworkPolicyClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeStagedNetworkPolicyClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.StagedNetworkPolicy, error) {
	return nil, nil
}

func (f *fakeStagedNetworkPolicyClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.StagedNetworkPolicyList, error) {
	return nil, nil
}

func (f *fakeStagedNetworkPolicyClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeStagedNetworkPolicyClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.StagedNetworkPolicy, err error) {
	return nil, nil
}

type fakeTierClient struct{}

func (f *fakeTierClient) Create(ctx context.Context, tier *v3.Tier, opts metav1.CreateOptions) (*v3.Tier, error) {
	return nil, nil
}

func (f *fakeTierClient) Update(ctx context.Context, tier *v3.Tier, opts metav1.UpdateOptions) (*v3.Tier, error) {
	return nil, nil
}

func (f *fakeTierClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (f *fakeTierClient) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (f *fakeTierClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v3.Tier, error) {
	return nil, nil
}

func (f *fakeTierClient) List(ctx context.Context, opts metav1.ListOptions) (*v3.TierList, error) {
	return nil, nil
}

func (f *fakeTierClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeTierClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v3.Tier, err error) {
	return nil, nil
}

type fakeRESTInterfaceClient struct{}

func (f *fakeRESTInterfaceClient) GetRateLimiter() flowcontrol.RateLimiter {
	return nil
}

func (f *fakeRESTInterfaceClient) Verb(verb string) *rest.Request {
	return nil
}

func (f *fakeRESTInterfaceClient) Post() *rest.Request {
	return nil
}

func (f *fakeRESTInterfaceClient) Put() *rest.Request {
	return nil
}

func (f *fakeRESTInterfaceClient) Patch(pt types.PatchType) *rest.Request {
	return nil
}

func (f *fakeRESTInterfaceClient) Get() *rest.Request {
	return nil
}

func (f *fakeRESTInterfaceClient) Delete() *rest.Request {
	return nil
}

func (f *fakeRESTInterfaceClient) APIVersion() schema.GroupVersion {
	return schema.GroupVersion{}
}
