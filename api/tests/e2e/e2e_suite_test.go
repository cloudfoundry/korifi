package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"github.com/go-http-utils/headers"
	"github.com/hashicorp/go-uuid"
	"github.com/matt-royal/biloba"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

type hierarchicalNamespace struct {
	label     string
	createdAt string
	guid      string
	children  []hierarchicalNamespace
}

var (
	testServerAddress  string
	k8sClient          client.Client
	rootNamespace      string
	apiServerRoot      string
	serviceAccountName string
	authHeader         string
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "E2E Suite", biloba.GoLandReporter())
}

var _ = BeforeSuite(func() {
	SetDefaultEventuallyTimeout(30 * time.Second)
	apiServerRoot = mustHaveEnv("API_SERVER_ROOT")

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	hnsv1alpha2.AddToScheme(scheme.Scheme)

	config, err := controllerruntime.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	rootNamespace = mustHaveEnv("ROOT_NAMESPACE")
	ensureServerIsUp()
})

var _ = BeforeEach(func() {
	serviceAccountName = generateGUID("user")
	token := obtainServiceAccountToken(serviceAccountName)
	authHeader = fmt.Sprintf("Bearer %s", token)
})

var _ = AfterEach(func() {
	deleteServiceAccount(serviceAccountName)
})

func mustHaveEnv(key string) string {
	val, ok := os.LookupEnv(key)
	Expect(ok).To(BeTrue(), "must set env var %q", key)

	return val
}

func ensureServerIsUp() {
	Eventually(func() (int, error) {
		resp, err := http.Get(apiServerRoot)
		if err != nil {
			return 0, err
		}

		resp.Body.Close()

		return resp.StatusCode, nil
	}, "30s").Should(Equal(http.StatusOK), "API Server at %s was not running after 30 seconds", apiServerRoot)
}

func generateGUID(prefix string) string {
	guid, err := uuid.GenerateUUID()
	Expect(err).NotTo(HaveOccurred())

	return fmt.Sprintf("%s-%s", prefix, guid[:6])
}

func waitForSubnamespaceAnchor(parent, name string) {
	Eventually(func() (bool, error) {
		anchor := &hnsv1alpha2.SubnamespaceAnchor{}
		err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: parent, Name: name}, anchor)
		if err != nil {
			return false, err
		}

		return anchor.Status.State == hnsv1alpha2.Ok, nil
	}, "30s").Should(BeTrue())
}

func waitForNamespaceDeletion(parent, ns string) {
	Eventually(func() (bool, error) {
		err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: parent, Name: ns}, &hnsv1alpha2.SubnamespaceAnchor{})
		if errors.IsNotFound(err) {
			return true, nil
		}

		return false, err
	}, "30s").Should(BeTrue())
}

func createHierarchicalNamespace(parentName, cfName, labelKey string) hierarchicalNamespace {
	ctx := context.Background()

	anchor := &hnsv1alpha2.SubnamespaceAnchor{}
	anchor.GenerateName = cfName
	anchor.Namespace = parentName
	anchor.Labels = map[string]string{labelKey: cfName}
	err := k8sClient.Create(ctx, anchor)
	Expect(err).NotTo(HaveOccurred())

	return hierarchicalNamespace{
		label:     cfName,
		guid:      anchor.Name,
		createdAt: anchor.CreationTimestamp.Time.UTC().Format(time.RFC3339),
	}
}

func deleteSubnamespace(parent, name string) {
	ctx := context.Background()

	anchor := hnsv1alpha2.SubnamespaceAnchor{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: parent,
			Name:      name,
		},
	}
	err := k8sClient.Delete(ctx, &anchor)
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() bool {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&anchor), &anchor)

		return errors.IsNotFound(err)
	}).Should(BeTrue())
}

func deleteSubnamespaceByLabel(parentNS, label string) {
	ctx := context.Background()

	err := k8sClient.DeleteAllOf(ctx, &hnsv1alpha2.SubnamespaceAnchor{}, client.MatchingLabels{repositories.OrgNameLabel: label}, client.InNamespace(parentNS))
	Expect(err).NotTo(HaveOccurred())
}

func createOrg(orgName string) presenter.OrgResponse {
	orgsUrl := apiServerRoot + "/v3/organizations"
	body := fmt.Sprintf(`{ "name": "%s" }`, orgName)
	req, err := http.NewRequest(http.MethodPost, orgsUrl, strings.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Add(headers.Authorization, authHeader)

	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()

	org := presenter.OrgResponse{}
	err = json.NewDecoder(resp.Body).Decode(&org)
	Expect(err).NotTo(HaveOccurred())

	return org
}

func createSpace(spaceName, orgGUID string) presenter.SpaceResponse {
	spacesURL := apiServerRoot + "/v3/spaces"
	body := fmt.Sprintf(`{
                "name": "%s",
                "relationships": {
                  "organization": {
                    "data": {
                      "guid": "%s"
                    }
                  }
                }
            }`, spaceName, orgGUID)
	req, err := http.NewRequest(http.MethodPost, spacesURL, strings.NewReader(body))
	Expect(err).NotTo(HaveOccurred())

	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()

	space := presenter.SpaceResponse{}
	err = json.NewDecoder(resp.Body).Decode(&space)
	Expect(err).NotTo(HaveOccurred())

	return space
}

func obtainServiceAccountToken(name string) string {
	var err error

	serviceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rootNamespace,
		},
	}
	err = k8sClient.Create(context.Background(), &serviceAccount)
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() error {
		if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&serviceAccount), &serviceAccount); err != nil {
			return err
		}

		if len(serviceAccount.Secrets) != 1 {
			return fmt.Errorf("expected exactly 1 secret, got %d", len(serviceAccount.Secrets))
		}

		return nil
	}, "120s").Should(Succeed())

	tokenSecret := corev1.Secret{}
	Eventually(func() error {
		return k8sClient.Get(context.Background(), client.ObjectKey{Name: serviceAccount.Secrets[0].Name, Namespace: rootNamespace}, &tokenSecret)
	}).Should(Succeed())

	return string(tokenSecret.Data["token"])
}

func deleteServiceAccount(name string) {
	serviceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rootNamespace,
		},
	}

	Expect(k8sClient.Delete(context.Background(), &serviceAccount)).To(Succeed())
}
