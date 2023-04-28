package repositories

import (
	"context"
	"errors"
	"fmt"
	"io"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/tools/image"
	"github.com/google/go-containerregistry/pkg/name"

	authv1 "k8s.io/api/authorization/v1"
	k8sclient "k8s.io/client-go/kubernetes"
)

//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get,namespace=ROOT_NAMESPACE
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get,namespace=ROOT_NAMESPACE

const SourceImageResourceType = "SourceImage"

//counterfeiter:generate -o fake -fake-name ImagePusher . ImagePusher

type ImagePusher interface {
	Push(ctx context.Context, creds image.Creds, repoRef string, zipReader io.Reader, tags ...string) (string, error)
}

type ImageRepository struct {
	privilegedK8sClient k8sclient.Interface
	userClientFactory   authorization.UserK8sClientFactory
	pusher              ImagePusher
	pushSecretName      string
	pushSecretNamespace string
}

func NewImageRepository(
	privilegedK8sClient k8sclient.Interface,
	userClientFactory authorization.UserK8sClientFactory,
	pusher ImagePusher,
	pushSecretName string,
	pushSecretNamespace string,
) *ImageRepository {
	return &ImageRepository{
		privilegedK8sClient: privilegedK8sClient,
		userClientFactory:   userClientFactory,
		pusher:              pusher,
		pushSecretName:      pushSecretName,
		pushSecretNamespace: pushSecretNamespace,
	}
}

func (r *ImageRepository) UploadSourceImage(ctx context.Context, authInfo authorization.Info, imageRef string, srcReader io.Reader, spaceGUID string, tags ...string) (string, error) {
	authorized, err := r.canIPatchCFPackage(ctx, authInfo, spaceGUID)
	if err != nil {
		return "", fmt.Errorf("checking auth to upload source image for failed: %w", err)
	}

	if !authorized {
		return "", apierrors.NewForbiddenError(errors.New("not authorized to patch cfpackage"), PackageResourceType)
	}

	_, err = name.ParseReference(imageRef)
	if err != nil {
		return "", apierrors.NewUnprocessableEntityError(err, fmt.Sprintf("invalid image ref: %q", imageRef))
	}

	pushedRef, err := r.pusher.Push(ctx, image.Creds{
		Namespace:  r.pushSecretNamespace,
		SecretName: r.pushSecretName,
	}, imageRef, srcReader, tags...)
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
