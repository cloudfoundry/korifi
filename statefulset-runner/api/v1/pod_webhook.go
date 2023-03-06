/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"context"
	"fmt"
	"regexp"

	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var podlog = logf.Log.WithName("pod-resource")

type STSPodDefaulter struct{}

func NewSTSPodDefaulter() *STSPodDefaulter {
	return &STSPodDefaulter{}
}

func (r *STSPodDefaulter) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&corev1.Pod{}).
		WithDefaulter(r).
		Complete()
}

// Mutate path is found here: https://github.com/kubernetes-sigs/controller-runtime/blob/15154aaa67679df320008ed45534f83ff3d6922d/pkg/builder/webhook.go#L201-L204
//+kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=fail,sideEffects=None,groups="",resources=pods,verbs=create,versions=v1,name=mstspod.korifi.cloudfoundry.org,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &STSPodDefaulter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *STSPodDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	podlog.V(1).Info("default", "name", obj.DeepCopyObject().GetObjectKind())

	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Pod but got a %T", obj))
	}

	index, err := parseAppIndex(pod.Name)
	if err != nil {
		return err
	}

	for c := range pod.Spec.Containers {
		container := &pod.Spec.Containers[c]
		if container.Name == controllers.ApplicationContainerName {
			cfInstanceVar := corev1.EnvVar{Name: controllers.EnvCFInstanceIndex, Value: index}
			container.Env = append(container.Env, cfInstanceVar)

			podlog.V(1).Info(fmt.Sprintf("patching-instance-index env-var - %s: %s", controllers.EnvCFInstanceIndex, index))

			return nil
		}
	}

	return nil
}

func parseAppIndex(podName string) (string, error) {
	expression := `-(\d+)$`
	r := regexp.MustCompile(expression)
	match := r.FindStringSubmatch(podName)

	if len(match) == 0 {
		return "", fmt.Errorf("pod %s name does not contain an index", podName)
	}

	return match[1], nil
}
