package migration

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const MigratedByLabelKey = "korifi.cloudfoundry.org/migrated-by"

var korifiObjectLists = []client.ObjectList{
	&korifiv1alpha1.CFRouteList{},
}

type Migrator struct {
	k8sClient     client.Client
	korifiVersion string
	workersCount  int
}

func New(k8sClient client.Client, korifiVersion string, workersCount int) *Migrator {
	return &Migrator{
		k8sClient:     k8sClient,
		korifiVersion: korifiVersion,
		workersCount:  workersCount,
	}
}

func (m *Migrator) Run(ctx context.Context) error {
	startTime := time.Now()

	objectsToUpdate, err := m.collectObjects(ctx, korifiObjectLists)
	if err != nil {
		panic(err)
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
				if err := m.setMigratedByLabel(ctx, obj); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to set label on object %v %s/%s: %v\n", obj.GetObjectKind(), obj.GetNamespace(), obj.GetName(), err)
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
