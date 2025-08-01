package securitygroups

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	v3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	"github.com/projectcalico/api/pkg/client/clientset_generated/clientset"
	"github.com/projectcalico/api/pkg/lib/numorstring"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Reconciler struct {
	k8sClient     client.Client
	calicoClient  clientset.Interface
	log           logr.Logger
	rootNamespace string
}

func NewReconciler(
	client client.Client,
	calicoClient clientset.Interface,
	log logr.Logger,
	rootNamespace string,
) *k8s.PatchingReconciler[korifiv1alpha1.CFSecurityGroup] {
	return k8s.NewPatchingReconciler(log, client, &Reconciler{
		k8sClient:     client,
		calicoClient:  calicoClient,
		log:           log,
		rootNamespace: rootNamespace,
	})
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFSecurityGroup{}).
		Named("cfsecuritygroup")
}

// +kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfsecuritygroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfsecuritygroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfsecuritygroups/finalizers,verbs=update
// +kubebuilder:rbac:groups=projectcalico.org,resources=networkpolicies;globalnetworkpolicies;tiers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=projectcalico.org,resources=tier.networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=projectcalico.org,resources=tier.globalnetworkpolicies,verbs=get;list;watch;create;update;patch;delete

func (r *Reconciler) ReconcileResource(ctx context.Context, sg *korifiv1alpha1.CFSecurityGroup) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	sg.Status.ObservedGeneration = sg.Generation
	log.V(1).Info("set observed generation", "generation", sg.Status.ObservedGeneration)

	if !sg.GetDeletionTimestamp().IsZero() {
		return r.finalizeCFSecurityGroup(ctx, sg)
	}

	if len(sg.Spec.Spaces) > 0 {
		sg.Labels = tools.SetMapValue(sg.Labels, korifiv1alpha1.CFSecurityGroupTypeLabel, korifiv1alpha1.CFSecurityGroupTypeSpaceScoped)
		if err := r.reconcileNetworkPolicies(ctx, sg); err != nil {
			return ctrl.Result{}, err
		}
	}

	if sg.Spec.GloballyEnabled.Running || sg.Spec.GloballyEnabled.Staging {
		sg.Labels = tools.SetMapValue(sg.Labels, korifiv1alpha1.CFSecurityGroupTypeLabel, korifiv1alpha1.CFSecurityGroupTypeGlobal)
		if err := r.reconcileGlobalNetworkPolicies(ctx, sg); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.cleanOrphanedPolicies(ctx, sg); err != nil {
		return ctrl.Result{}, err
	}

	log.V(1).Info("CFSecurityGroup reconciled")
	return ctrl.Result{}, nil
}

func (r *Reconciler) finalizeCFSecurityGroup(ctx context.Context, sg *korifiv1alpha1.CFSecurityGroup) (ctrl.Result, error) {
	logs := logr.FromContextOrDiscard(ctx).WithName("finalize-security-group")

	policies, err := r.calicoClient.ProjectcalicoV3().NetworkPolicies("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", fmt.Sprintf("default.%s", sg.Name)),
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list NetworkPolicies: %w", err)
	}

	for _, policy := range policies.Items {
		if err = r.k8sClient.Delete(ctx, &policy); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete NetworkPolicy %s: %w", policy.Name, err)
		}
	}

	globalPolicies, err := r.calicoClient.ProjectcalicoV3().GlobalNetworkPolicies().List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", fmt.Sprintf("default.%s", sg.Name)),
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list NetworkPolicies: %w", err)
	}

	for _, globalPolicy := range globalPolicies.Items {
		if err := r.k8sClient.Delete(ctx, &globalPolicy); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete NetworkPolicy %s: %w", globalPolicy.Name, err)
		}
	}

	if controllerutil.RemoveFinalizer(sg, korifiv1alpha1.CFSecurityGroupFinalizerName) {
		logs.V(1).Info("finalizer removed")
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) cleanOrphanedPolicies(ctx context.Context, sg *korifiv1alpha1.CFSecurityGroup) error {
	policies, err := r.calicoClient.ProjectcalicoV3().NetworkPolicies("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", fmt.Sprintf("default.%s", sg.Name)),
		LabelSelector: fmt.Sprintf("%s=%s",
			korifiv1alpha1.CFSecurityGroupTypeLabel, korifiv1alpha1.CFSecurityGroupTypeSpaceScoped),
	})
	if err != nil {
		return fmt.Errorf("failed to list NetworkPolicies: %w", err)
	}

	for _, policy := range policies.Items {
		if _, exists := sg.Spec.Spaces[policy.Namespace]; !exists {
			if err = r.calicoClient.ProjectcalicoV3().NetworkPolicies(policy.Namespace).Delete(ctx, policy.Name, metav1.DeleteOptions{}); err != nil {
				return fmt.Errorf("failed to delete orphaned NetworkPolicy %s/%s: %w", policy.Namespace, policy.Name, err)
			}
		}
	}

	if !sg.Spec.GloballyEnabled.Running && !sg.Spec.GloballyEnabled.Staging {
		globalPolicy, err := r.calicoClient.ProjectcalicoV3().GlobalNetworkPolicies().Get(ctx, fmt.Sprintf("default.%s", sg.Name), metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}

			return fmt.Errorf("failed to get GlobalNetworkPolicy %s: %w", sg.Name, err)
		}

		if err = r.calicoClient.ProjectcalicoV3().GlobalNetworkPolicies().Delete(ctx, globalPolicy.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete orphaned GlobalNetworkPolicy %s: %w", globalPolicy.Name, err)
		}
	}

	return nil
}

func (r *Reconciler) reconcileNetworkPolicies(ctx context.Context, sg *korifiv1alpha1.CFSecurityGroup) error {
	for space, workloads := range sg.Spec.Spaces {
		if err := r.reconcileNetworkPolicyForSpace(ctx, sg, space, workloads); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) reconcileNetworkPolicyForSpace(ctx context.Context, sg *korifiv1alpha1.CFSecurityGroup, space string, workloads korifiv1alpha1.SecurityGroupWorkloads) error {
	policy, err := r.calicoClient.ProjectcalicoV3().NetworkPolicies(space).Get(ctx, fmt.Sprintf("default.%s", sg.Name), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.createNetworkPolicy(ctx, sg, space, workloads)
		}
		return fmt.Errorf("failed to get NetworkPolicy %s/%s: %w", space, sg.Name, err)
	}

	err = securityGroupToNetworkPolicy(sg, workloads, policy)
	if err != nil {
		return err
	}

	if _, err := r.calicoClient.ProjectcalicoV3().NetworkPolicies(space).Update(ctx, policy, metav1.UpdateOptions{}); err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) reconcileGlobalNetworkPolicies(ctx context.Context, sg *korifiv1alpha1.CFSecurityGroup) error {
	policy, err := r.calicoClient.ProjectcalicoV3().GlobalNetworkPolicies().Get(ctx, fmt.Sprintf("default.%s", sg.Name), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.createGlobalNetworkPolicy(ctx, sg)
		}
		return fmt.Errorf("failed to get NetworkPolicy %s: %w", sg.Name, err)
	}

	err = r.securityGroupToGlobalNetworkPolicy(sg, policy)
	if err != nil {
		return err
	}

	policy.Labels = tools.SetMapValue(policy.Labels, korifiv1alpha1.CFSecurityGroupTypeLabel, sg.Labels[korifiv1alpha1.CFSecurityGroupTypeLabel])
	if _, err := r.calicoClient.ProjectcalicoV3().GlobalNetworkPolicies().Update(ctx, policy, metav1.UpdateOptions{}); err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) createNetworkPolicy(ctx context.Context, sg *korifiv1alpha1.CFSecurityGroup, space string, workloads korifiv1alpha1.SecurityGroupWorkloads) error {
	policy := &v3.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sg.Name,
			Namespace: space,
		},
	}

	err := securityGroupToNetworkPolicy(sg, workloads, policy)
	if err != nil {
		return err
	}

	policy.Labels = tools.SetMapValue(policy.Labels, korifiv1alpha1.CFSecurityGroupTypeLabel, sg.Labels[korifiv1alpha1.CFSecurityGroupTypeLabel])
	if _, err = r.calicoClient.ProjectcalicoV3().NetworkPolicies(space).Create(ctx, policy, metav1.CreateOptions{}); err != nil {
		r.log.Error(err, "failed to create NetworkPolicy", "namespace", space, "name", sg.Name)
		return err
	}

	return nil
}

func (r *Reconciler) createGlobalNetworkPolicy(ctx context.Context, sg *korifiv1alpha1.CFSecurityGroup) error {
	policy := &v3.GlobalNetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: sg.Name,
		},
	}

	err := r.securityGroupToGlobalNetworkPolicy(sg, policy)
	if err != nil {
		return err
	}

	policy.Labels = tools.SetMapValue(policy.Labels, korifiv1alpha1.CFSecurityGroupTypeLabel, sg.Labels[korifiv1alpha1.CFSecurityGroupTypeLabel])
	if _, err = r.calicoClient.ProjectcalicoV3().GlobalNetworkPolicies().Create(ctx, policy, metav1.CreateOptions{}); err != nil {
		r.log.Error(err, "failed to create NetworkPolicy", "name", sg.Name)
		return err
	}

	return nil
}

func securityGroupToNetworkPolicy(sg *korifiv1alpha1.CFSecurityGroup, workloads korifiv1alpha1.SecurityGroupWorkloads, policy *v3.NetworkPolicy) error {
	egressRules, err := buildEgressRules(sg.Spec.Rules)
	if err != nil {
		return err
	}

	policy.Spec.Egress = egressRules
	policy.Spec.Selector = buildSelector(workloads)
	policy.Spec.Types = []v3.PolicyType{v3.PolicyTypeEgress}

	return nil
}

func (r *Reconciler) securityGroupToGlobalNetworkPolicy(sg *korifiv1alpha1.CFSecurityGroup, policy *v3.GlobalNetworkPolicy) error {
	egressRules, err := buildEgressRules(sg.Spec.Rules)
	if err != nil {
		return err
	}

	policy.Spec.Egress = egressRules
	policy.Spec.Selector = buildSelector(sg.Spec.GloballyEnabled)
	policy.Spec.Types = []v3.PolicyType{v3.PolicyTypeEgress}
	policy.Spec.NamespaceSelector = "has(korifi.cloudfoundry.org/space-guid)"

	return nil
}

func buildEgressRules(rules []korifiv1alpha1.SecurityGroupRule) ([]v3.Rule, error) {
	var egressRules []v3.Rule

	for _, rule := range rules {
		nets, err := buildRuleNets(rule)
		if err != nil {
			return []v3.Rule{}, err
		}

		ports, err := buildRulePorts(rule)
		if err != nil {
			return []v3.Rule{}, err
		}

		egressRules = append(egressRules, v3.Rule{
			Action:   v3.Allow,
			Protocol: &numorstring.Protocol{Type: 1, StrVal: getProtocol(rule.Protocol)},
			Destination: v3.EntityRule{
				Nets:  nets,
				Ports: ports,
			},
		})
	}

	return egressRules, nil
}

func buildRulePorts(rule korifiv1alpha1.SecurityGroupRule) ([]numorstring.Port, error) {
	var ports []numorstring.Port

	if strings.Contains(rule.Ports, "-") {
		port, err := parseRangePorts(rule.Ports)
		if err != nil {
			return nil, err
		}

		return []numorstring.Port{port}, nil
	}

	for _, portStr := range strings.Split(rule.Ports, ",") {
		port, err := portStringToUint16(portStr)
		if err != nil {
			return nil, err
		}

		ports = append(ports, numorstring.Port{MinPort: port, MaxPort: port})
	}

	return ports, nil
}

func buildSelector(workloads korifiv1alpha1.SecurityGroupWorkloads) string {
	var workloadTypes []string

	if workloads.Running {
		workloadTypes = append(workloadTypes, korifiv1alpha1.CFWorkloadTypeApp)
	}
	if workloads.Staging {
		workloadTypes = append(workloadTypes, korifiv1alpha1.CFWorkloadTypeBuild)
	}
	values := "'" + strings.Join(workloadTypes, "', '") + "'"

	return fmt.Sprintf("%s in { %s }", korifiv1alpha1.CFWorkloadTypeLabelkey, values)
}

func parseRangePorts(ports string) (numorstring.Port, error) {
	rangePorts := strings.Split(ports, "-")
	if len(rangePorts) != 2 {
		return numorstring.Port{}, fmt.Errorf("invalid port range format: %s", ports)
	}

	start, err := portStringToUint16(rangePorts[0])
	if err != nil {
		return numorstring.Port{}, err
	}

	end, err := portStringToUint16(rangePorts[1])
	if err != nil {
		return numorstring.Port{}, err
	}

	return numorstring.Port{
		MinPort: start,
		MaxPort: end,
	}, nil
}

func portStringToUint16(port string) (uint16, error) {
	pStr := strings.TrimSpace(port)
	if pStr == "" {
		return 0, fmt.Errorf("port value cannot be empty")
	}
	p, err := strconv.ParseUint(pStr, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid port %s: %w", pStr, err)
	}
	return uint16(p), nil
}

func buildRuleNets(rule korifiv1alpha1.SecurityGroupRule) ([]string, error) {
	if strings.Contains(rule.Destination, "-") {
		var nets []string
		parts := strings.Split(rule.Destination, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid IP range format: %s", rule.Destination)
		}

		cidrs, err := generateCIDRs(parts[0], parts[1])
		if err != nil {
			return nil, err
		}
		nets = append(nets, cidrs...)

		return nets, nil
	}

	if _, _, err := net.ParseCIDR(rule.Destination); err == nil {
		return []string{rule.Destination}, nil
	}

	if ip := net.ParseIP(rule.Destination); ip != nil {
		return []string{fmt.Sprintf("%s/32", rule.Destination)}, nil
	}

	return nil, fmt.Errorf("invalid destination: %s", rule.Destination)
}

func generateCIDRs(startIP, endIP string) ([]string, error) {
	start, err := ipToUint32(startIP)
	if err != nil {
		return nil, err
	}

	end, err := ipToUint32(endIP)
	if err != nil {
		return nil, err
	}

	if start > end {
		return nil, fmt.Errorf("start IP %s must be less than or equal to end IP %s", startIP, endIP)
	}

	var cidrs []string
	for end >= start {
		mask := uint32(0xFFFFFFFF)
		length := 32

		for mask > 0 {
			nextMask := mask << 1
			if (start&nextMask) != start || (start|^nextMask) > end {
				break
			}
			mask = nextMask
			length--
		}

		cidrs = append(cidrs, fmt.Sprintf("%s/%d", uint32ToIP(start), length))

		start |= ^mask
		if start+1 < start { // Handle overflow
			break
		}
		start++
	}

	return cidrs, nil
}

func ipToUint32(ip string) (uint32, error) {
	parsedIP := net.ParseIP(ip).To4()
	if parsedIP == nil {
		return 0, fmt.Errorf("invalid IPv4 address: %s", ip)
	}
	return uint32(parsedIP[0])<<24 | uint32(parsedIP[1])<<16 | uint32(parsedIP[2])<<8 | uint32(parsedIP[3]), nil
}

func uint32ToIP(ip uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", (ip>>24)&0xFF, (ip>>16)&0xFF, (ip>>8)&0xFF, ip&0xFF)
}

func getProtocol(protocol string) string {
	switch protocol {
	case korifiv1alpha1.ProtocolTCP:
		return numorstring.ProtocolTCP
	case korifiv1alpha1.ProtocolUDP:
		return numorstring.ProtocolUDP
	default:
		return numorstring.ProtocolTCP
	}
}
