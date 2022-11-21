/*
Copyright 2021.

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

package workloads

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-cftask,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cftasks/status,verbs=create;update,versions=v1alpha1,name=mcftask.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

var cfTaskLog = logf.Log.WithName("cftask-resource")

type CFTaskDefaulter struct{}

func NewCFTaskDefaulter() *CFTaskDefaulter {
	return &CFTaskDefaulter{}
}

func (d *CFTaskDefaulter) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&korifiv1alpha1.CFTask{}).
		WithDefaulter(d).
		Complete()
}

func (d *CFTaskDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	cfTaskLog.Info("Mutating CFTask webhook handler")

	cfTask, ok := obj.(*korifiv1alpha1.CFTask)
	if !ok {
		return fmt.Errorf("object %v is not a CFTask", obj)
	}

	if cfTask.Status.SequenceID != 0 {
		return nil
	}

	seqId, err := generateSequenceId()
	if err != nil {
		return err
	}

	cfTask.Status.SequenceID = seqId
	return nil
}

func generateSequenceId() (int64, error) {
	now := time.Now()

	// This is a bit annoying. Golang's builtin support for millis in
	// time.Format() would always put a dot (or comma) before the millis. We do
	// not need them for sequence IDs that we want to be in the format of
	// YYYYMMDDhhmmssSSS
	seqIdString := strings.ReplaceAll(now.Format("20060102150405.000"), ".", "")

	return strconv.ParseInt(seqIdString, 10, 64)
}
