package repositories

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"

	registryv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/pivotal/kpack/pkg/dockercreds/k8sdockercreds"
	kpackregistry "github.com/pivotal/kpack/pkg/registry"

	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	k8sclient "k8s.io/client-go/kubernetes"
)

//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get

const SourceImageResourceType = "SourceImage"

//counterfeiter:generate -o fake -fake-name ImageBuilder . ImageBuilder
//counterfeiter:generate -o fake -fake-name ImagePusher . ImagePusher

type ImageBuilder interface {
	Build(ctx context.Context, srcReader io.Reader) (registryv1.Image, error)
}

type ImagePusher interface {
	Push(imageRef string, image registryv1.Image, credentials remote.Option, transport remote.Option) (string, error)
}

type ImageRepository struct {
	privilegedK8sClient k8sclient.Interface
	userClientFactory   authorization.UserK8sClientFactory
	rootNamespace       string
	registrySecretName  string
	registryCAPath      string

	builder ImageBuilder
	pusher  ImagePusher
}

func NewImageRepository(
	privilegedK8sClient k8sclient.Interface,
	userClientFactory authorization.UserK8sClientFactory,
	rootNamespace,
	registrySecretName string,
	registryCAPath string,
	builder ImageBuilder,
	pusher ImagePusher,
) *ImageRepository {
	return &ImageRepository{
		privilegedK8sClient: privilegedK8sClient,
		userClientFactory:   userClientFactory,
		rootNamespace:       rootNamespace,
		registrySecretName:  registrySecretName,
		registryCAPath:      registryCAPath,
		builder:             builder,
		pusher:              pusher,
	}
}

func (r *ImageRepository) UploadSourceImage(ctx context.Context, authInfo authorization.Info, imageRef string, srcReader io.Reader, spaceGUID string) (string, error) {
	authorized, err := r.canIPatchCFPackage(ctx, authInfo, spaceGUID)
	if err != nil {
		return "", fmt.Errorf("checking auth to upload source image for failed: %w", err)
	}

	if !authorized {
		return "", apierrors.NewForbiddenError(errors.New("not authorized to patch cfpackage"), PackageResourceType)
	}

	image, err := r.builder.Build(ctx, srcReader)
	if err != nil {
		return "", fmt.Errorf("image build for ref '%s' failed: %w", imageRef, err)
	}

	credentials, err := r.getCredentials(ctx)
	if err != nil {
		return "", fmt.Errorf("getting push credentials for image ref '%s' failed: %w", imageRef, err)
	}

	transport, err := configureTransport(r.registryCAPath)
	if err != nil {
		return "", fmt.Errorf("configuring transport for image ref '%s' failed: %w", imageRef, err)
	}

	pushedRef, err := r.pusher.Push(imageRef, image, credentials, transport)
	if err != nil {
		return "", apierrors.NewBlobstoreUnavailableError(fmt.Errorf("pushing image ref '%s' failed: %w", imageRef, err))
	}

	return pushedRef, nil
}

func (r *ImageRepository) canIPatchCFPackage(ctx context.Context, authInfo authorization.Info, spaceGUID string) (bool, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return false, fmt.Errorf("canIPatchCFPackage: failed to create user k8s client: %w", err)
	}

	review := authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authv1.ResourceAttributes{
				Namespace: spaceGUID,
				Verb:      "patch",
				Group:     "korifi.cloudfoundry.org",
				Resource:  "cfpackages",
			},
		},
	}
	if err := userClient.Create(ctx, &review); err != nil {
		return false, fmt.Errorf("canIPatchCFPackage: failed to create self subject access review: %w", apierrors.FromK8sError(err, PackageResourceType))
	}

	return review.Status.Allowed, nil
}

func (r *ImageRepository) getCredentials(ctx context.Context) (remote.Option, error) {
	keychainFactory, err := k8sdockercreds.NewSecretKeychainFactory(r.privilegedK8sClient)
	if err != nil {
		return nil, fmt.Errorf("error in k8sdockercreds.NewSecretKeychainFactory: %w", apierrors.FromK8sError(err, SourceImageResourceType))
	}
	keychain, err := keychainFactory.KeychainForSecretRef(ctx, kpackregistry.SecretRef{
		Namespace:        r.rootNamespace,
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: r.registrySecretName}},
	})
	if err != nil {
		return nil, fmt.Errorf("error in keychainFactory.KeychainForSecretRef: %w", apierrors.FromK8sError(err, SourceImageResourceType))
	}

	return remote.WithAuthFromKeychain(keychain), nil
}

func configureTransport(caCertPath string) (remote.Option, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}

	if caCertPath != "" {
		var pemCerts []byte
		if pemCerts, err = os.ReadFile(caCertPath); err != nil {
			return nil, err
		} else if ok := pool.AppendCertsFromPEM(pemCerts); !ok {
			return nil, errors.New("failed to append k8s cert bundle to cert pool")
		}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    pool,
	}

	return remote.WithTransport(transport), nil
}
