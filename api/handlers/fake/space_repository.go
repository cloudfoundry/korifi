// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type SpaceRepository struct {
	CreateSpaceStub        func(context.Context, authorization.Info, repositories.CreateSpaceMessage) (repositories.SpaceRecord, error)
	createSpaceMutex       sync.RWMutex
	createSpaceArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.CreateSpaceMessage
	}
	createSpaceReturns struct {
		result1 repositories.SpaceRecord
		result2 error
	}
	createSpaceReturnsOnCall map[int]struct {
		result1 repositories.SpaceRecord
		result2 error
	}
	DeleteSpaceStub        func(context.Context, authorization.Info, repositories.DeleteSpaceMessage) error
	deleteSpaceMutex       sync.RWMutex
	deleteSpaceArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.DeleteSpaceMessage
	}
	deleteSpaceReturns struct {
		result1 error
	}
	deleteSpaceReturnsOnCall map[int]struct {
		result1 error
	}
	GetSpaceStub        func(context.Context, authorization.Info, string) (repositories.SpaceRecord, error)
	getSpaceMutex       sync.RWMutex
	getSpaceArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
	}
	getSpaceReturns struct {
		result1 repositories.SpaceRecord
		result2 error
	}
	getSpaceReturnsOnCall map[int]struct {
		result1 repositories.SpaceRecord
		result2 error
	}
	ListSpacesStub        func(context.Context, authorization.Info, repositories.ListSpacesMessage) ([]repositories.SpaceRecord, error)
	listSpacesMutex       sync.RWMutex
	listSpacesArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListSpacesMessage
	}
	listSpacesReturns struct {
		result1 []repositories.SpaceRecord
		result2 error
	}
	listSpacesReturnsOnCall map[int]struct {
		result1 []repositories.SpaceRecord
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *SpaceRepository) CreateSpace(arg1 context.Context, arg2 authorization.Info, arg3 repositories.CreateSpaceMessage) (repositories.SpaceRecord, error) {
	fake.createSpaceMutex.Lock()
	ret, specificReturn := fake.createSpaceReturnsOnCall[len(fake.createSpaceArgsForCall)]
	fake.createSpaceArgsForCall = append(fake.createSpaceArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.CreateSpaceMessage
	}{arg1, arg2, arg3})
	stub := fake.CreateSpaceStub
	fakeReturns := fake.createSpaceReturns
	fake.recordInvocation("CreateSpace", []interface{}{arg1, arg2, arg3})
	fake.createSpaceMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *SpaceRepository) CreateSpaceCallCount() int {
	fake.createSpaceMutex.RLock()
	defer fake.createSpaceMutex.RUnlock()
	return len(fake.createSpaceArgsForCall)
}

func (fake *SpaceRepository) CreateSpaceCalls(stub func(context.Context, authorization.Info, repositories.CreateSpaceMessage) (repositories.SpaceRecord, error)) {
	fake.createSpaceMutex.Lock()
	defer fake.createSpaceMutex.Unlock()
	fake.CreateSpaceStub = stub
}

func (fake *SpaceRepository) CreateSpaceArgsForCall(i int) (context.Context, authorization.Info, repositories.CreateSpaceMessage) {
	fake.createSpaceMutex.RLock()
	defer fake.createSpaceMutex.RUnlock()
	argsForCall := fake.createSpaceArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *SpaceRepository) CreateSpaceReturns(result1 repositories.SpaceRecord, result2 error) {
	fake.createSpaceMutex.Lock()
	defer fake.createSpaceMutex.Unlock()
	fake.CreateSpaceStub = nil
	fake.createSpaceReturns = struct {
		result1 repositories.SpaceRecord
		result2 error
	}{result1, result2}
}

func (fake *SpaceRepository) CreateSpaceReturnsOnCall(i int, result1 repositories.SpaceRecord, result2 error) {
	fake.createSpaceMutex.Lock()
	defer fake.createSpaceMutex.Unlock()
	fake.CreateSpaceStub = nil
	if fake.createSpaceReturnsOnCall == nil {
		fake.createSpaceReturnsOnCall = make(map[int]struct {
			result1 repositories.SpaceRecord
			result2 error
		})
	}
	fake.createSpaceReturnsOnCall[i] = struct {
		result1 repositories.SpaceRecord
		result2 error
	}{result1, result2}
}

func (fake *SpaceRepository) DeleteSpace(arg1 context.Context, arg2 authorization.Info, arg3 repositories.DeleteSpaceMessage) error {
	fake.deleteSpaceMutex.Lock()
	ret, specificReturn := fake.deleteSpaceReturnsOnCall[len(fake.deleteSpaceArgsForCall)]
	fake.deleteSpaceArgsForCall = append(fake.deleteSpaceArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.DeleteSpaceMessage
	}{arg1, arg2, arg3})
	stub := fake.DeleteSpaceStub
	fakeReturns := fake.deleteSpaceReturns
	fake.recordInvocation("DeleteSpace", []interface{}{arg1, arg2, arg3})
	fake.deleteSpaceMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *SpaceRepository) DeleteSpaceCallCount() int {
	fake.deleteSpaceMutex.RLock()
	defer fake.deleteSpaceMutex.RUnlock()
	return len(fake.deleteSpaceArgsForCall)
}

func (fake *SpaceRepository) DeleteSpaceCalls(stub func(context.Context, authorization.Info, repositories.DeleteSpaceMessage) error) {
	fake.deleteSpaceMutex.Lock()
	defer fake.deleteSpaceMutex.Unlock()
	fake.DeleteSpaceStub = stub
}

func (fake *SpaceRepository) DeleteSpaceArgsForCall(i int) (context.Context, authorization.Info, repositories.DeleteSpaceMessage) {
	fake.deleteSpaceMutex.RLock()
	defer fake.deleteSpaceMutex.RUnlock()
	argsForCall := fake.deleteSpaceArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *SpaceRepository) DeleteSpaceReturns(result1 error) {
	fake.deleteSpaceMutex.Lock()
	defer fake.deleteSpaceMutex.Unlock()
	fake.DeleteSpaceStub = nil
	fake.deleteSpaceReturns = struct {
		result1 error
	}{result1}
}

func (fake *SpaceRepository) DeleteSpaceReturnsOnCall(i int, result1 error) {
	fake.deleteSpaceMutex.Lock()
	defer fake.deleteSpaceMutex.Unlock()
	fake.DeleteSpaceStub = nil
	if fake.deleteSpaceReturnsOnCall == nil {
		fake.deleteSpaceReturnsOnCall = make(map[int]struct {
			result1 error
		})
	}
	fake.deleteSpaceReturnsOnCall[i] = struct {
		result1 error
	}{result1}
}

func (fake *SpaceRepository) GetSpace(arg1 context.Context, arg2 authorization.Info, arg3 string) (repositories.SpaceRecord, error) {
	fake.getSpaceMutex.Lock()
	ret, specificReturn := fake.getSpaceReturnsOnCall[len(fake.getSpaceArgsForCall)]
	fake.getSpaceArgsForCall = append(fake.getSpaceArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
	}{arg1, arg2, arg3})
	stub := fake.GetSpaceStub
	fakeReturns := fake.getSpaceReturns
	fake.recordInvocation("GetSpace", []interface{}{arg1, arg2, arg3})
	fake.getSpaceMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *SpaceRepository) GetSpaceCallCount() int {
	fake.getSpaceMutex.RLock()
	defer fake.getSpaceMutex.RUnlock()
	return len(fake.getSpaceArgsForCall)
}

func (fake *SpaceRepository) GetSpaceCalls(stub func(context.Context, authorization.Info, string) (repositories.SpaceRecord, error)) {
	fake.getSpaceMutex.Lock()
	defer fake.getSpaceMutex.Unlock()
	fake.GetSpaceStub = stub
}

func (fake *SpaceRepository) GetSpaceArgsForCall(i int) (context.Context, authorization.Info, string) {
	fake.getSpaceMutex.RLock()
	defer fake.getSpaceMutex.RUnlock()
	argsForCall := fake.getSpaceArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *SpaceRepository) GetSpaceReturns(result1 repositories.SpaceRecord, result2 error) {
	fake.getSpaceMutex.Lock()
	defer fake.getSpaceMutex.Unlock()
	fake.GetSpaceStub = nil
	fake.getSpaceReturns = struct {
		result1 repositories.SpaceRecord
		result2 error
	}{result1, result2}
}

func (fake *SpaceRepository) GetSpaceReturnsOnCall(i int, result1 repositories.SpaceRecord, result2 error) {
	fake.getSpaceMutex.Lock()
	defer fake.getSpaceMutex.Unlock()
	fake.GetSpaceStub = nil
	if fake.getSpaceReturnsOnCall == nil {
		fake.getSpaceReturnsOnCall = make(map[int]struct {
			result1 repositories.SpaceRecord
			result2 error
		})
	}
	fake.getSpaceReturnsOnCall[i] = struct {
		result1 repositories.SpaceRecord
		result2 error
	}{result1, result2}
}

func (fake *SpaceRepository) ListSpaces(arg1 context.Context, arg2 authorization.Info, arg3 repositories.ListSpacesMessage) ([]repositories.SpaceRecord, error) {
	fake.listSpacesMutex.Lock()
	ret, specificReturn := fake.listSpacesReturnsOnCall[len(fake.listSpacesArgsForCall)]
	fake.listSpacesArgsForCall = append(fake.listSpacesArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListSpacesMessage
	}{arg1, arg2, arg3})
	stub := fake.ListSpacesStub
	fakeReturns := fake.listSpacesReturns
	fake.recordInvocation("ListSpaces", []interface{}{arg1, arg2, arg3})
	fake.listSpacesMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *SpaceRepository) ListSpacesCallCount() int {
	fake.listSpacesMutex.RLock()
	defer fake.listSpacesMutex.RUnlock()
	return len(fake.listSpacesArgsForCall)
}

func (fake *SpaceRepository) ListSpacesCalls(stub func(context.Context, authorization.Info, repositories.ListSpacesMessage) ([]repositories.SpaceRecord, error)) {
	fake.listSpacesMutex.Lock()
	defer fake.listSpacesMutex.Unlock()
	fake.ListSpacesStub = stub
}

func (fake *SpaceRepository) ListSpacesArgsForCall(i int) (context.Context, authorization.Info, repositories.ListSpacesMessage) {
	fake.listSpacesMutex.RLock()
	defer fake.listSpacesMutex.RUnlock()
	argsForCall := fake.listSpacesArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *SpaceRepository) ListSpacesReturns(result1 []repositories.SpaceRecord, result2 error) {
	fake.listSpacesMutex.Lock()
	defer fake.listSpacesMutex.Unlock()
	fake.ListSpacesStub = nil
	fake.listSpacesReturns = struct {
		result1 []repositories.SpaceRecord
		result2 error
	}{result1, result2}
}

func (fake *SpaceRepository) ListSpacesReturnsOnCall(i int, result1 []repositories.SpaceRecord, result2 error) {
	fake.listSpacesMutex.Lock()
	defer fake.listSpacesMutex.Unlock()
	fake.ListSpacesStub = nil
	if fake.listSpacesReturnsOnCall == nil {
		fake.listSpacesReturnsOnCall = make(map[int]struct {
			result1 []repositories.SpaceRecord
			result2 error
		})
	}
	fake.listSpacesReturnsOnCall[i] = struct {
		result1 []repositories.SpaceRecord
		result2 error
	}{result1, result2}
}

func (fake *SpaceRepository) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.createSpaceMutex.RLock()
	defer fake.createSpaceMutex.RUnlock()
	fake.deleteSpaceMutex.RLock()
	defer fake.deleteSpaceMutex.RUnlock()
	fake.getSpaceMutex.RLock()
	defer fake.getSpaceMutex.RUnlock()
	fake.listSpacesMutex.RLock()
	defer fake.listSpacesMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *SpaceRepository) recordInvocation(key string, args []interface{}) {
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

var _ handlers.SpaceRepository = new(SpaceRepository)
