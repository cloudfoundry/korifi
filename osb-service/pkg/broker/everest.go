package broker

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var databaseClusterGVR = schema.GroupVersionResource{
	Group:    "everest.percona.com",
	Version:  "v1alpha1",
	Resource: "databaseclusters",
}

type everestClient struct {
	dyn          dynamic.Interface
	core         kubernetes.Interface
	namespace    string
	hostNS       string
	vclusterName string
}

func newEverestClient(o Options) (*everestClient, error) {
	if o.EverestKubeconfig == "" {
		return nil, nil
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", o.EverestKubeconfig)
	if err != nil {
		return nil, fmt.Errorf("everest kubeconfig: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	core, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &everestClient{
		dyn:          dyn,
		core:         core,
		namespace:    o.EverestNamespace,
		hostNS:       o.EverestHostNamespace,
		vclusterName: o.EverestVClusterName,
	}, nil
}

func (c *everestClient) healthy(ctx context.Context) error {
	_, err := c.dyn.Resource(databaseClusterGVR).Namespace(c.namespace).List(ctx, metav1.ListOptions{Limit: 1})
	return err
}

func (c *everestClient) provision(ctx context.Context, instanceID string) (map[string]any, error) {
	name, err := resourceName("p", instanceID)
	if err != nil {
		return nil, err
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "everest.percona.com/v1alpha1",
		"kind":       "DatabaseCluster",
		"metadata": map[string]any{
			"name":      name,
			"namespace": c.namespace,
			"labels": map[string]any{
				"osb.korifi/instance-id": instanceID,
			},
		},
		"spec": map[string]any{
			"backup": map[string]any{"pitr": map[string]any{"enabled": false}},
			"engine": map[string]any{
				"type":     "postgresql",
				"replicas": 1,
				"version":  "17.4",
				"storage":  map[string]any{"size": "2Gi"},
				"resources": map[string]any{
					"cpu":    "250m",
					"memory": "512Mi",
				},
			},
			"proxy": map[string]any{
				"type":     "pgbouncer",
				"replicas": 1,
				"expose":   map[string]any{"type": "internal"},
			},
		},
	}}
	_, err = c.dyn.Resource(databaseClusterGVR).Namespace(c.namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("create DatabaseCluster: %w", err)
	}
	secret, err := c.waitUserSecret(ctx, name)
	if err != nil {
		return nil, err
	}
	return c.credentialsFromSecret(name, secret), nil
}

func (c *everestClient) deprovision(ctx context.Context, name string) error {
	err := c.dyn.Resource(databaseClusterGVR).Namespace(c.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *everestClient) waitUserSecret(ctx context.Context, name string) (*corev1.Secret, error) {
	candidates := []string{name + "-pguser-" + name, "everest-secrets-" + name}
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		for _, secretName := range candidates {
			sec, err := c.core.CoreV1().Secrets(c.namespace).Get(ctx, secretName, metav1.GetOptions{})
			if err == nil && len(sec.Data["password"]) > 0 {
				return sec, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return nil, fmt.Errorf("timed out waiting for OpenEverest user secret for %s", name)
}

func (c *everestClient) credentialsFromSecret(cluster string, sec *corev1.Secret) map[string]any {
	get := func(keys ...string) string {
		for _, k := range keys {
			if v := sec.Data[k]; len(v) > 0 {
				return string(v)
			}
		}
		return ""
	}
	user := get("user", "username")
	pass := get("password")
	db := get("dbname", "database")
	if db == "" {
		db = cluster
	}
	port := 5432
	if p := get("port"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	host := fmt.Sprintf("%s-primary-%s-x-%s.%s.svc.cluster.local", cluster, c.namespace, c.vclusterName, c.hostNS)
	creds := postgresCredentials(host, port, "require", db, user, pass)
	creds["cluster"] = cluster
	return creds
}
