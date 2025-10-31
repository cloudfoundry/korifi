package migration

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/coordination"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const MigratedByLabelKey = "korifi.cloudfoundry.org/migrated-by"

var korifiObjectLists = []client.ObjectList{
	&korifiv1alpha1.CFServiceBindingList{},
}

type Migrator struct {
	k8sClient                  client.Client
	korifiVersion              string
	workersCount               int
	serviceBindingNameRegistry coordination.NameRegistry
}

func New(k8sClient client.Client, korifiVersion string, workersCount int, serviceBindingNameRegistry coordination.NameRegistry) *Migrator {
	return &Migrator{
		k8sClient:                  k8sClient,
		korifiVersion:              korifiVersion,
		workersCount:               workersCount,
		serviceBindingNameRegistry: serviceBindingNameRegistry,
	}
}

func (m *Migrator) Run(ctx context.Context) error {
	startTime := time.Now()

	objectsToUpdate, err := m.collectObjects(ctx, korifiObjectLists)
	if err != nil {
		return fmt.Errorf("failed to collect objects to migrate: %v", err)
	}

	fmt.Println("==========================================================")
	fmt.Fprintf(os.Stdout, "Using %d workers to migrate %d objects to version %s\n", m.workersCount, len(objectsToUpdate), m.korifiVersion)
	fmt.Println("==========================================================")
	fmt.Println("")

	wg := sync.WaitGroup{}
	wg.Add(m.workersCount)

	objectChan := make(chan client.Object, len(objectsToUpdate))
	for _, obj := range objectsToUpdate {
		objectChan <- obj
	}

	for i := 0; i < m.workersCount; i++ {
		go func() {
			defer wg.Done()

			for obj := range objectChan {
				binding, ok := obj.(*korifiv1alpha1.CFServiceBinding)
				if !ok {
					continue
				}

				if binding.Labels[MigratedByLabelKey] == m.korifiVersion {
					continue
				}

				if err := m.migrateServiceBindingUniqueName(ctx, binding); err != nil {
					fmt.Fprintf(os.Stderr, "failed to migrate lease for service binding %s/%s: %v\n", binding.Namespace, binding.Name, err)
					continue
				}
				fmt.Fprintf(os.Stdout, "%s %s/%s migrated\n", obj.GetObjectKind().GroupVersionKind().GroupKind().Kind, obj.GetNamespace(), obj.GetName())
			}
		}()
	}

	close(objectChan)
	wg.Wait()
	fmt.Println("")
	fmt.Println("==========================================================")
	fmt.Fprintf(os.Stdout, "Migration completed successfully, took %s!\n", time.Since(startTime))

	return nil
}

func (m *Migrator) migrateServiceBindingUniqueName(ctx context.Context, binding *korifiv1alpha1.CFServiceBinding) error {
	err := m.registerServiceBindingUniqueName(ctx, binding)
	if err != nil {
		return fmt.Errorf("failed to register the new unique name for service binding %s/%s: %v", binding.Namespace, binding.Name, err)
	}

	err = m.serviceBindingNameRegistry.DeregisterName(ctx, binding.Namespace, oldUniqueName(binding))
	if err != nil {
		return fmt.Errorf("failed to unregister old unique name for service binding %s/%s: %v", binding.Namespace, binding.Name, err)
	}

	return m.setMigratedByLabel(ctx, binding)
}

func (m *Migrator) registerServiceBindingUniqueName(ctx context.Context, binding *korifiv1alpha1.CFServiceBinding) error {
	isOwned, err := m.serviceBindingNameRegistry.CheckNameOwnership(ctx, binding.Namespace, binding.UniqueName(), binding.Namespace, binding.Name)
	// NotFound means that there is no lease for that unique name
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to check ownership of the new binding unique name:  %w", err)
	}

	if isOwned {
		return nil
	}
	return m.serviceBindingNameRegistry.RegisterName(ctx, binding.Namespace, binding.UniqueName(), binding.Namespace, binding.Name)
}

func oldUniqueName(binding *korifiv1alpha1.CFServiceBinding) string {
	return fmt.Sprintf("sb::%s::%s::%s", binding.Spec.AppRef.Name, binding.Spec.Service.Namespace, binding.Spec.Service.Name)
}

func (m *Migrator) setMigratedByLabel(ctx context.Context, obj client.Object) error {
	return k8s.PatchResource(ctx, m.k8sClient, obj, func() {
		obj.SetLabels(tools.SetMapValue(obj.GetLabels(), MigratedByLabelKey, m.korifiVersion))
	})
}

func (m *Migrator) collectObjects(ctx context.Context, objectLists []client.ObjectList) ([]client.Object, error) {
	var objects []client.Object
	for _, list := range objectLists {
		typedObjects, err := m.collectObjectsByType(ctx, list)
		if err != nil {
			return nil, err
		}
		objects = append(objects, typedObjects...)
	}
	return objects, nil
}

func (m *Migrator) collectObjectsByType(ctx context.Context, objectList client.ObjectList) ([]client.Object, error) {
	if err := m.k8sClient.List(ctx, objectList); err != nil {
		return nil, fmt.Errorf("failed to list %T: %v", objectList, err)
	}

	s := reflect.ValueOf(objectList).Elem().FieldByName("Items")
	objects := make([]client.Object, s.Len())
	for i := 0; i < s.Len(); i++ {
		objects[i] = s.Index(i).Addr().Interface().(client.Object)
	}
	return objects, nil
}
