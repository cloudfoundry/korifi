package broker

import "context"

type memoryBackend struct {
	instances map[string]instance
	bindings  map[string]BindResponse
}

func newMemoryBackend() *memoryBackend {
	return &memoryBackend{
		instances: make(map[string]instance),
		bindings:  make(map[string]BindResponse),
	}
}

func (m *memoryBackend) getInstance(id string) (instance, bool, error) {
	i, ok := m.instances[id]
	return i, ok, nil
}

func (m *memoryBackend) putInstance(id string, inst instance) error {
	m.instances[id] = inst
	return nil
}

func (m *memoryBackend) deleteInstance(id string) (bool, error) {
	if _, ok := m.instances[id]; !ok {
		return false, nil
	}
	delete(m.instances, id)
	for key := range m.bindings {
		if len(key) > len(id) && key[:len(id)+1] == id+"/" {
			delete(m.bindings, key)
		}
	}
	return true, nil
}

func (m *memoryBackend) getBinding(instanceID, bindingID string) (BindResponse, bool, error) {
	v, ok := m.bindings[instanceID+"/"+bindingID]
	return v, ok, nil
}

func (m *memoryBackend) putBinding(instanceID, bindingID string, resp BindResponse) error {
	m.bindings[instanceID+"/"+bindingID] = resp
	return nil
}

func (m *memoryBackend) deleteBinding(instanceID, bindingID string) (bool, error) {
	key := instanceID + "/" + bindingID
	if _, ok := m.bindings[key]; !ok {
		return false, nil
	}
	delete(m.bindings, key)
	return true, nil
}

func (m *memoryBackend) healthy(context.Context) error { return nil }
