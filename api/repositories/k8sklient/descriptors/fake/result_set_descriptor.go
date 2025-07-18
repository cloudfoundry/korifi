// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"sync"

	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
)

type ResultSetDescriptor struct {
	GUIDsStub        func() ([]string, error)
	gUIDsMutex       sync.RWMutex
	gUIDsArgsForCall []struct {
	}
	gUIDsReturns struct {
		result1 []string
		result2 error
	}
	gUIDsReturnsOnCall map[int]struct {
		result1 []string
		result2 error
	}
	SortStub        func(string, bool) error
	sortMutex       sync.RWMutex
	sortArgsForCall []struct {
		arg1 string
		arg2 bool
	}
	sortReturns struct {
		result1 error
	}
	sortReturnsOnCall map[int]struct {
		result1 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *ResultSetDescriptor) GUIDs() ([]string, error) {
	fake.gUIDsMutex.Lock()
	ret, specificReturn := fake.gUIDsReturnsOnCall[len(fake.gUIDsArgsForCall)]
	fake.gUIDsArgsForCall = append(fake.gUIDsArgsForCall, struct {
	}{})
	stub := fake.GUIDsStub
	fakeReturns := fake.gUIDsReturns
	fake.recordInvocation("GUIDs", []interface{}{})
	fake.gUIDsMutex.Unlock()
	if stub != nil {
		return stub()
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *ResultSetDescriptor) GUIDsCallCount() int {
	fake.gUIDsMutex.RLock()
	defer fake.gUIDsMutex.RUnlock()
	return len(fake.gUIDsArgsForCall)
}

func (fake *ResultSetDescriptor) GUIDsCalls(stub func() ([]string, error)) {
	fake.gUIDsMutex.Lock()
	defer fake.gUIDsMutex.Unlock()
	fake.GUIDsStub = stub
}

func (fake *ResultSetDescriptor) GUIDsReturns(result1 []string, result2 error) {
	fake.gUIDsMutex.Lock()
	defer fake.gUIDsMutex.Unlock()
	fake.GUIDsStub = nil
	fake.gUIDsReturns = struct {
		result1 []string
		result2 error
	}{result1, result2}
}

func (fake *ResultSetDescriptor) GUIDsReturnsOnCall(i int, result1 []string, result2 error) {
	fake.gUIDsMutex.Lock()
	defer fake.gUIDsMutex.Unlock()
	fake.GUIDsStub = nil
	if fake.gUIDsReturnsOnCall == nil {
		fake.gUIDsReturnsOnCall = make(map[int]struct {
			result1 []string
			result2 error
		})
	}
	fake.gUIDsReturnsOnCall[i] = struct {
		result1 []string
		result2 error
	}{result1, result2}
}

func (fake *ResultSetDescriptor) Sort(arg1 string, arg2 bool) error {
	fake.sortMutex.Lock()
	ret, specificReturn := fake.sortReturnsOnCall[len(fake.sortArgsForCall)]
	fake.sortArgsForCall = append(fake.sortArgsForCall, struct {
		arg1 string
		arg2 bool
	}{arg1, arg2})
	stub := fake.SortStub
	fakeReturns := fake.sortReturns
	fake.recordInvocation("Sort", []interface{}{arg1, arg2})
	fake.sortMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *ResultSetDescriptor) SortCallCount() int {
	fake.sortMutex.RLock()
	defer fake.sortMutex.RUnlock()
	return len(fake.sortArgsForCall)
}

func (fake *ResultSetDescriptor) SortCalls(stub func(string, bool) error) {
	fake.sortMutex.Lock()
	defer fake.sortMutex.Unlock()
	fake.SortStub = stub
}

func (fake *ResultSetDescriptor) SortArgsForCall(i int) (string, bool) {
	fake.sortMutex.RLock()
	defer fake.sortMutex.RUnlock()
	argsForCall := fake.sortArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2
}

func (fake *ResultSetDescriptor) SortReturns(result1 error) {
	fake.sortMutex.Lock()
	defer fake.sortMutex.Unlock()
	fake.SortStub = nil
	fake.sortReturns = struct {
		result1 error
	}{result1}
}

func (fake *ResultSetDescriptor) SortReturnsOnCall(i int, result1 error) {
	fake.sortMutex.Lock()
	defer fake.sortMutex.Unlock()
	fake.SortStub = nil
	if fake.sortReturnsOnCall == nil {
		fake.sortReturnsOnCall = make(map[int]struct {
			result1 error
		})
	}
	fake.sortReturnsOnCall[i] = struct {
		result1 error
	}{result1}
}

func (fake *ResultSetDescriptor) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *ResultSetDescriptor) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ descriptors.ResultSetDescriptor = new(ResultSetDescriptor)
