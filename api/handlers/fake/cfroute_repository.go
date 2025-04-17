// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type CFRouteRepository struct {
	AddDestinationsToRouteStub        func(context.Context, authorization.Info, repositories.AddDestinationsMessage) (repositories.RouteRecord, error)
	addDestinationsToRouteMutex       sync.RWMutex
	addDestinationsToRouteArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.AddDestinationsMessage
	}
	addDestinationsToRouteReturns struct {
		result1 repositories.RouteRecord
		result2 error
	}
	addDestinationsToRouteReturnsOnCall map[int]struct {
		result1 repositories.RouteRecord
		result2 error
	}
	CreateRouteStub        func(context.Context, authorization.Info, repositories.CreateRouteMessage) (repositories.RouteRecord, error)
	createRouteMutex       sync.RWMutex
	createRouteArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.CreateRouteMessage
	}
	createRouteReturns struct {
		result1 repositories.RouteRecord
		result2 error
	}
	createRouteReturnsOnCall map[int]struct {
		result1 repositories.RouteRecord
		result2 error
	}
	DeleteRouteStub        func(context.Context, authorization.Info, repositories.DeleteRouteMessage) error
	deleteRouteMutex       sync.RWMutex
	deleteRouteArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.DeleteRouteMessage
	}
	deleteRouteReturns struct {
		result1 error
	}
	deleteRouteReturnsOnCall map[int]struct {
		result1 error
	}
	DeleteUnmappedRoutesStub        func(context.Context, authorization.Info, string) error
	deleteUnmappedRoutesMutex       sync.RWMutex
	deleteUnmappedRoutesArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
	}
	deleteUnmappedRoutesReturns struct {
		result1 error
	}
	deleteUnmappedRoutesReturnsOnCall map[int]struct {
		result1 error
	}
	GetRouteStub        func(context.Context, authorization.Info, string) (repositories.RouteRecord, error)
	getRouteMutex       sync.RWMutex
	getRouteArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
	}
	getRouteReturns struct {
		result1 repositories.RouteRecord
		result2 error
	}
	getRouteReturnsOnCall map[int]struct {
		result1 repositories.RouteRecord
		result2 error
	}
	ListRoutesStub        func(context.Context, authorization.Info, repositories.ListRoutesMessage) ([]repositories.RouteRecord, error)
	listRoutesMutex       sync.RWMutex
	listRoutesArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListRoutesMessage
	}
	listRoutesReturns struct {
		result1 []repositories.RouteRecord
		result2 error
	}
	listRoutesReturnsOnCall map[int]struct {
		result1 []repositories.RouteRecord
		result2 error
	}
	ListRoutesForAppStub        func(context.Context, authorization.Info, string, string) ([]repositories.RouteRecord, error)
	listRoutesForAppMutex       sync.RWMutex
	listRoutesForAppArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
		arg4 string
	}
	listRoutesForAppReturns struct {
		result1 []repositories.RouteRecord
		result2 error
	}
	listRoutesForAppReturnsOnCall map[int]struct {
		result1 []repositories.RouteRecord
		result2 error
	}
	PatchRouteMetadataStub        func(context.Context, authorization.Info, repositories.PatchRouteMetadataMessage) (repositories.RouteRecord, error)
	patchRouteMetadataMutex       sync.RWMutex
	patchRouteMetadataArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.PatchRouteMetadataMessage
	}
	patchRouteMetadataReturns struct {
		result1 repositories.RouteRecord
		result2 error
	}
	patchRouteMetadataReturnsOnCall map[int]struct {
		result1 repositories.RouteRecord
		result2 error
	}
	RemoveDestinationFromRouteStub        func(context.Context, authorization.Info, repositories.RemoveDestinationMessage) (repositories.RouteRecord, error)
	removeDestinationFromRouteMutex       sync.RWMutex
	removeDestinationFromRouteArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.RemoveDestinationMessage
	}
	removeDestinationFromRouteReturns struct {
		result1 repositories.RouteRecord
		result2 error
	}
	removeDestinationFromRouteReturnsOnCall map[int]struct {
		result1 repositories.RouteRecord
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *CFRouteRepository) AddDestinationsToRoute(arg1 context.Context, arg2 authorization.Info, arg3 repositories.AddDestinationsMessage) (repositories.RouteRecord, error) {
	fake.addDestinationsToRouteMutex.Lock()
	ret, specificReturn := fake.addDestinationsToRouteReturnsOnCall[len(fake.addDestinationsToRouteArgsForCall)]
	fake.addDestinationsToRouteArgsForCall = append(fake.addDestinationsToRouteArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.AddDestinationsMessage
	}{arg1, arg2, arg3})
	stub := fake.AddDestinationsToRouteStub
	fakeReturns := fake.addDestinationsToRouteReturns
	fake.recordInvocation("AddDestinationsToRoute", []interface{}{arg1, arg2, arg3})
	fake.addDestinationsToRouteMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *CFRouteRepository) AddDestinationsToRouteCallCount() int {
	fake.addDestinationsToRouteMutex.RLock()
	defer fake.addDestinationsToRouteMutex.RUnlock()
	return len(fake.addDestinationsToRouteArgsForCall)
}

func (fake *CFRouteRepository) AddDestinationsToRouteCalls(stub func(context.Context, authorization.Info, repositories.AddDestinationsMessage) (repositories.RouteRecord, error)) {
	fake.addDestinationsToRouteMutex.Lock()
	defer fake.addDestinationsToRouteMutex.Unlock()
	fake.AddDestinationsToRouteStub = stub
}

func (fake *CFRouteRepository) AddDestinationsToRouteArgsForCall(i int) (context.Context, authorization.Info, repositories.AddDestinationsMessage) {
	fake.addDestinationsToRouteMutex.RLock()
	defer fake.addDestinationsToRouteMutex.RUnlock()
	argsForCall := fake.addDestinationsToRouteArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFRouteRepository) AddDestinationsToRouteReturns(result1 repositories.RouteRecord, result2 error) {
	fake.addDestinationsToRouteMutex.Lock()
	defer fake.addDestinationsToRouteMutex.Unlock()
	fake.AddDestinationsToRouteStub = nil
	fake.addDestinationsToRouteReturns = struct {
		result1 repositories.RouteRecord
		result2 error
	}{result1, result2}
}

func (fake *CFRouteRepository) AddDestinationsToRouteReturnsOnCall(i int, result1 repositories.RouteRecord, result2 error) {
	fake.addDestinationsToRouteMutex.Lock()
	defer fake.addDestinationsToRouteMutex.Unlock()
	fake.AddDestinationsToRouteStub = nil
	if fake.addDestinationsToRouteReturnsOnCall == nil {
		fake.addDestinationsToRouteReturnsOnCall = make(map[int]struct {
			result1 repositories.RouteRecord
			result2 error
		})
	}
	fake.addDestinationsToRouteReturnsOnCall[i] = struct {
		result1 repositories.RouteRecord
		result2 error
	}{result1, result2}
}

func (fake *CFRouteRepository) CreateRoute(arg1 context.Context, arg2 authorization.Info, arg3 repositories.CreateRouteMessage) (repositories.RouteRecord, error) {
	fake.createRouteMutex.Lock()
	ret, specificReturn := fake.createRouteReturnsOnCall[len(fake.createRouteArgsForCall)]
	fake.createRouteArgsForCall = append(fake.createRouteArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.CreateRouteMessage
	}{arg1, arg2, arg3})
	stub := fake.CreateRouteStub
	fakeReturns := fake.createRouteReturns
	fake.recordInvocation("CreateRoute", []interface{}{arg1, arg2, arg3})
	fake.createRouteMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *CFRouteRepository) CreateRouteCallCount() int {
	fake.createRouteMutex.RLock()
	defer fake.createRouteMutex.RUnlock()
	return len(fake.createRouteArgsForCall)
}

func (fake *CFRouteRepository) CreateRouteCalls(stub func(context.Context, authorization.Info, repositories.CreateRouteMessage) (repositories.RouteRecord, error)) {
	fake.createRouteMutex.Lock()
	defer fake.createRouteMutex.Unlock()
	fake.CreateRouteStub = stub
}

func (fake *CFRouteRepository) CreateRouteArgsForCall(i int) (context.Context, authorization.Info, repositories.CreateRouteMessage) {
	fake.createRouteMutex.RLock()
	defer fake.createRouteMutex.RUnlock()
	argsForCall := fake.createRouteArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFRouteRepository) CreateRouteReturns(result1 repositories.RouteRecord, result2 error) {
	fake.createRouteMutex.Lock()
	defer fake.createRouteMutex.Unlock()
	fake.CreateRouteStub = nil
	fake.createRouteReturns = struct {
		result1 repositories.RouteRecord
		result2 error
	}{result1, result2}
}

func (fake *CFRouteRepository) CreateRouteReturnsOnCall(i int, result1 repositories.RouteRecord, result2 error) {
	fake.createRouteMutex.Lock()
	defer fake.createRouteMutex.Unlock()
	fake.CreateRouteStub = nil
	if fake.createRouteReturnsOnCall == nil {
		fake.createRouteReturnsOnCall = make(map[int]struct {
			result1 repositories.RouteRecord
			result2 error
		})
	}
	fake.createRouteReturnsOnCall[i] = struct {
		result1 repositories.RouteRecord
		result2 error
	}{result1, result2}
}

func (fake *CFRouteRepository) DeleteRoute(arg1 context.Context, arg2 authorization.Info, arg3 repositories.DeleteRouteMessage) error {
	fake.deleteRouteMutex.Lock()
	ret, specificReturn := fake.deleteRouteReturnsOnCall[len(fake.deleteRouteArgsForCall)]
	fake.deleteRouteArgsForCall = append(fake.deleteRouteArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.DeleteRouteMessage
	}{arg1, arg2, arg3})
	stub := fake.DeleteRouteStub
	fakeReturns := fake.deleteRouteReturns
	fake.recordInvocation("DeleteRoute", []interface{}{arg1, arg2, arg3})
	fake.deleteRouteMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *CFRouteRepository) DeleteRouteCallCount() int {
	fake.deleteRouteMutex.RLock()
	defer fake.deleteRouteMutex.RUnlock()
	return len(fake.deleteRouteArgsForCall)
}

func (fake *CFRouteRepository) DeleteRouteCalls(stub func(context.Context, authorization.Info, repositories.DeleteRouteMessage) error) {
	fake.deleteRouteMutex.Lock()
	defer fake.deleteRouteMutex.Unlock()
	fake.DeleteRouteStub = stub
}

func (fake *CFRouteRepository) DeleteRouteArgsForCall(i int) (context.Context, authorization.Info, repositories.DeleteRouteMessage) {
	fake.deleteRouteMutex.RLock()
	defer fake.deleteRouteMutex.RUnlock()
	argsForCall := fake.deleteRouteArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFRouteRepository) DeleteRouteReturns(result1 error) {
	fake.deleteRouteMutex.Lock()
	defer fake.deleteRouteMutex.Unlock()
	fake.DeleteRouteStub = nil
	fake.deleteRouteReturns = struct {
		result1 error
	}{result1}
}

func (fake *CFRouteRepository) DeleteRouteReturnsOnCall(i int, result1 error) {
	fake.deleteRouteMutex.Lock()
	defer fake.deleteRouteMutex.Unlock()
	fake.DeleteRouteStub = nil
	if fake.deleteRouteReturnsOnCall == nil {
		fake.deleteRouteReturnsOnCall = make(map[int]struct {
			result1 error
		})
	}
	fake.deleteRouteReturnsOnCall[i] = struct {
		result1 error
	}{result1}
}

func (fake *CFRouteRepository) DeleteUnmappedRoutes(arg1 context.Context, arg2 authorization.Info, arg3 string) error {
	fake.deleteUnmappedRoutesMutex.Lock()
	ret, specificReturn := fake.deleteUnmappedRoutesReturnsOnCall[len(fake.deleteUnmappedRoutesArgsForCall)]
	fake.deleteUnmappedRoutesArgsForCall = append(fake.deleteUnmappedRoutesArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
	}{arg1, arg2, arg3})
	stub := fake.DeleteUnmappedRoutesStub
	fakeReturns := fake.deleteUnmappedRoutesReturns
	fake.recordInvocation("DeleteUnmappedRoutes", []interface{}{arg1, arg2, arg3})
	fake.deleteUnmappedRoutesMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *CFRouteRepository) DeleteUnmappedRoutesCallCount() int {
	fake.deleteUnmappedRoutesMutex.RLock()
	defer fake.deleteUnmappedRoutesMutex.RUnlock()
	return len(fake.deleteUnmappedRoutesArgsForCall)
}

func (fake *CFRouteRepository) DeleteUnmappedRoutesCalls(stub func(context.Context, authorization.Info, string) error) {
	fake.deleteUnmappedRoutesMutex.Lock()
	defer fake.deleteUnmappedRoutesMutex.Unlock()
	fake.DeleteUnmappedRoutesStub = stub
}

func (fake *CFRouteRepository) DeleteUnmappedRoutesArgsForCall(i int) (context.Context, authorization.Info, string) {
	fake.deleteUnmappedRoutesMutex.RLock()
	defer fake.deleteUnmappedRoutesMutex.RUnlock()
	argsForCall := fake.deleteUnmappedRoutesArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFRouteRepository) DeleteUnmappedRoutesReturns(result1 error) {
	fake.deleteUnmappedRoutesMutex.Lock()
	defer fake.deleteUnmappedRoutesMutex.Unlock()
	fake.DeleteUnmappedRoutesStub = nil
	fake.deleteUnmappedRoutesReturns = struct {
		result1 error
	}{result1}
}

func (fake *CFRouteRepository) DeleteUnmappedRoutesReturnsOnCall(i int, result1 error) {
	fake.deleteUnmappedRoutesMutex.Lock()
	defer fake.deleteUnmappedRoutesMutex.Unlock()
	fake.DeleteUnmappedRoutesStub = nil
	if fake.deleteUnmappedRoutesReturnsOnCall == nil {
		fake.deleteUnmappedRoutesReturnsOnCall = make(map[int]struct {
			result1 error
		})
	}
	fake.deleteUnmappedRoutesReturnsOnCall[i] = struct {
		result1 error
	}{result1}
}

func (fake *CFRouteRepository) GetRoute(arg1 context.Context, arg2 authorization.Info, arg3 string) (repositories.RouteRecord, error) {
	fake.getRouteMutex.Lock()
	ret, specificReturn := fake.getRouteReturnsOnCall[len(fake.getRouteArgsForCall)]
	fake.getRouteArgsForCall = append(fake.getRouteArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
	}{arg1, arg2, arg3})
	stub := fake.GetRouteStub
	fakeReturns := fake.getRouteReturns
	fake.recordInvocation("GetRoute", []interface{}{arg1, arg2, arg3})
	fake.getRouteMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *CFRouteRepository) GetRouteCallCount() int {
	fake.getRouteMutex.RLock()
	defer fake.getRouteMutex.RUnlock()
	return len(fake.getRouteArgsForCall)
}

func (fake *CFRouteRepository) GetRouteCalls(stub func(context.Context, authorization.Info, string) (repositories.RouteRecord, error)) {
	fake.getRouteMutex.Lock()
	defer fake.getRouteMutex.Unlock()
	fake.GetRouteStub = stub
}

func (fake *CFRouteRepository) GetRouteArgsForCall(i int) (context.Context, authorization.Info, string) {
	fake.getRouteMutex.RLock()
	defer fake.getRouteMutex.RUnlock()
	argsForCall := fake.getRouteArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFRouteRepository) GetRouteReturns(result1 repositories.RouteRecord, result2 error) {
	fake.getRouteMutex.Lock()
	defer fake.getRouteMutex.Unlock()
	fake.GetRouteStub = nil
	fake.getRouteReturns = struct {
		result1 repositories.RouteRecord
		result2 error
	}{result1, result2}
}

func (fake *CFRouteRepository) GetRouteReturnsOnCall(i int, result1 repositories.RouteRecord, result2 error) {
	fake.getRouteMutex.Lock()
	defer fake.getRouteMutex.Unlock()
	fake.GetRouteStub = nil
	if fake.getRouteReturnsOnCall == nil {
		fake.getRouteReturnsOnCall = make(map[int]struct {
			result1 repositories.RouteRecord
			result2 error
		})
	}
	fake.getRouteReturnsOnCall[i] = struct {
		result1 repositories.RouteRecord
		result2 error
	}{result1, result2}
}

func (fake *CFRouteRepository) ListRoutes(arg1 context.Context, arg2 authorization.Info, arg3 repositories.ListRoutesMessage) ([]repositories.RouteRecord, error) {
	fake.listRoutesMutex.Lock()
	ret, specificReturn := fake.listRoutesReturnsOnCall[len(fake.listRoutesArgsForCall)]
	fake.listRoutesArgsForCall = append(fake.listRoutesArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListRoutesMessage
	}{arg1, arg2, arg3})
	stub := fake.ListRoutesStub
	fakeReturns := fake.listRoutesReturns
	fake.recordInvocation("ListRoutes", []interface{}{arg1, arg2, arg3})
	fake.listRoutesMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *CFRouteRepository) ListRoutesCallCount() int {
	fake.listRoutesMutex.RLock()
	defer fake.listRoutesMutex.RUnlock()
	return len(fake.listRoutesArgsForCall)
}

func (fake *CFRouteRepository) ListRoutesCalls(stub func(context.Context, authorization.Info, repositories.ListRoutesMessage) ([]repositories.RouteRecord, error)) {
	fake.listRoutesMutex.Lock()
	defer fake.listRoutesMutex.Unlock()
	fake.ListRoutesStub = stub
}

func (fake *CFRouteRepository) ListRoutesArgsForCall(i int) (context.Context, authorization.Info, repositories.ListRoutesMessage) {
	fake.listRoutesMutex.RLock()
	defer fake.listRoutesMutex.RUnlock()
	argsForCall := fake.listRoutesArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFRouteRepository) ListRoutesReturns(result1 []repositories.RouteRecord, result2 error) {
	fake.listRoutesMutex.Lock()
	defer fake.listRoutesMutex.Unlock()
	fake.ListRoutesStub = nil
	fake.listRoutesReturns = struct {
		result1 []repositories.RouteRecord
		result2 error
	}{result1, result2}
}

func (fake *CFRouteRepository) ListRoutesReturnsOnCall(i int, result1 []repositories.RouteRecord, result2 error) {
	fake.listRoutesMutex.Lock()
	defer fake.listRoutesMutex.Unlock()
	fake.ListRoutesStub = nil
	if fake.listRoutesReturnsOnCall == nil {
		fake.listRoutesReturnsOnCall = make(map[int]struct {
			result1 []repositories.RouteRecord
			result2 error
		})
	}
	fake.listRoutesReturnsOnCall[i] = struct {
		result1 []repositories.RouteRecord
		result2 error
	}{result1, result2}
}

func (fake *CFRouteRepository) ListRoutesForApp(arg1 context.Context, arg2 authorization.Info, arg3 string, arg4 string) ([]repositories.RouteRecord, error) {
	fake.listRoutesForAppMutex.Lock()
	ret, specificReturn := fake.listRoutesForAppReturnsOnCall[len(fake.listRoutesForAppArgsForCall)]
	fake.listRoutesForAppArgsForCall = append(fake.listRoutesForAppArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
		arg4 string
	}{arg1, arg2, arg3, arg4})
	stub := fake.ListRoutesForAppStub
	fakeReturns := fake.listRoutesForAppReturns
	fake.recordInvocation("ListRoutesForApp", []interface{}{arg1, arg2, arg3, arg4})
	fake.listRoutesForAppMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3, arg4)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *CFRouteRepository) ListRoutesForAppCallCount() int {
	fake.listRoutesForAppMutex.RLock()
	defer fake.listRoutesForAppMutex.RUnlock()
	return len(fake.listRoutesForAppArgsForCall)
}

func (fake *CFRouteRepository) ListRoutesForAppCalls(stub func(context.Context, authorization.Info, string, string) ([]repositories.RouteRecord, error)) {
	fake.listRoutesForAppMutex.Lock()
	defer fake.listRoutesForAppMutex.Unlock()
	fake.ListRoutesForAppStub = stub
}

func (fake *CFRouteRepository) ListRoutesForAppArgsForCall(i int) (context.Context, authorization.Info, string, string) {
	fake.listRoutesForAppMutex.RLock()
	defer fake.listRoutesForAppMutex.RUnlock()
	argsForCall := fake.listRoutesForAppArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3, argsForCall.arg4
}

func (fake *CFRouteRepository) ListRoutesForAppReturns(result1 []repositories.RouteRecord, result2 error) {
	fake.listRoutesForAppMutex.Lock()
	defer fake.listRoutesForAppMutex.Unlock()
	fake.ListRoutesForAppStub = nil
	fake.listRoutesForAppReturns = struct {
		result1 []repositories.RouteRecord
		result2 error
	}{result1, result2}
}

func (fake *CFRouteRepository) ListRoutesForAppReturnsOnCall(i int, result1 []repositories.RouteRecord, result2 error) {
	fake.listRoutesForAppMutex.Lock()
	defer fake.listRoutesForAppMutex.Unlock()
	fake.ListRoutesForAppStub = nil
	if fake.listRoutesForAppReturnsOnCall == nil {
		fake.listRoutesForAppReturnsOnCall = make(map[int]struct {
			result1 []repositories.RouteRecord
			result2 error
		})
	}
	fake.listRoutesForAppReturnsOnCall[i] = struct {
		result1 []repositories.RouteRecord
		result2 error
	}{result1, result2}
}

func (fake *CFRouteRepository) PatchRouteMetadata(arg1 context.Context, arg2 authorization.Info, arg3 repositories.PatchRouteMetadataMessage) (repositories.RouteRecord, error) {
	fake.patchRouteMetadataMutex.Lock()
	ret, specificReturn := fake.patchRouteMetadataReturnsOnCall[len(fake.patchRouteMetadataArgsForCall)]
	fake.patchRouteMetadataArgsForCall = append(fake.patchRouteMetadataArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.PatchRouteMetadataMessage
	}{arg1, arg2, arg3})
	stub := fake.PatchRouteMetadataStub
	fakeReturns := fake.patchRouteMetadataReturns
	fake.recordInvocation("PatchRouteMetadata", []interface{}{arg1, arg2, arg3})
	fake.patchRouteMetadataMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *CFRouteRepository) PatchRouteMetadataCallCount() int {
	fake.patchRouteMetadataMutex.RLock()
	defer fake.patchRouteMetadataMutex.RUnlock()
	return len(fake.patchRouteMetadataArgsForCall)
}

func (fake *CFRouteRepository) PatchRouteMetadataCalls(stub func(context.Context, authorization.Info, repositories.PatchRouteMetadataMessage) (repositories.RouteRecord, error)) {
	fake.patchRouteMetadataMutex.Lock()
	defer fake.patchRouteMetadataMutex.Unlock()
	fake.PatchRouteMetadataStub = stub
}

func (fake *CFRouteRepository) PatchRouteMetadataArgsForCall(i int) (context.Context, authorization.Info, repositories.PatchRouteMetadataMessage) {
	fake.patchRouteMetadataMutex.RLock()
	defer fake.patchRouteMetadataMutex.RUnlock()
	argsForCall := fake.patchRouteMetadataArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFRouteRepository) PatchRouteMetadataReturns(result1 repositories.RouteRecord, result2 error) {
	fake.patchRouteMetadataMutex.Lock()
	defer fake.patchRouteMetadataMutex.Unlock()
	fake.PatchRouteMetadataStub = nil
	fake.patchRouteMetadataReturns = struct {
		result1 repositories.RouteRecord
		result2 error
	}{result1, result2}
}

func (fake *CFRouteRepository) PatchRouteMetadataReturnsOnCall(i int, result1 repositories.RouteRecord, result2 error) {
	fake.patchRouteMetadataMutex.Lock()
	defer fake.patchRouteMetadataMutex.Unlock()
	fake.PatchRouteMetadataStub = nil
	if fake.patchRouteMetadataReturnsOnCall == nil {
		fake.patchRouteMetadataReturnsOnCall = make(map[int]struct {
			result1 repositories.RouteRecord
			result2 error
		})
	}
	fake.patchRouteMetadataReturnsOnCall[i] = struct {
		result1 repositories.RouteRecord
		result2 error
	}{result1, result2}
}

func (fake *CFRouteRepository) RemoveDestinationFromRoute(arg1 context.Context, arg2 authorization.Info, arg3 repositories.RemoveDestinationMessage) (repositories.RouteRecord, error) {
	fake.removeDestinationFromRouteMutex.Lock()
	ret, specificReturn := fake.removeDestinationFromRouteReturnsOnCall[len(fake.removeDestinationFromRouteArgsForCall)]
	fake.removeDestinationFromRouteArgsForCall = append(fake.removeDestinationFromRouteArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.RemoveDestinationMessage
	}{arg1, arg2, arg3})
	stub := fake.RemoveDestinationFromRouteStub
	fakeReturns := fake.removeDestinationFromRouteReturns
	fake.recordInvocation("RemoveDestinationFromRoute", []interface{}{arg1, arg2, arg3})
	fake.removeDestinationFromRouteMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *CFRouteRepository) RemoveDestinationFromRouteCallCount() int {
	fake.removeDestinationFromRouteMutex.RLock()
	defer fake.removeDestinationFromRouteMutex.RUnlock()
	return len(fake.removeDestinationFromRouteArgsForCall)
}

func (fake *CFRouteRepository) RemoveDestinationFromRouteCalls(stub func(context.Context, authorization.Info, repositories.RemoveDestinationMessage) (repositories.RouteRecord, error)) {
	fake.removeDestinationFromRouteMutex.Lock()
	defer fake.removeDestinationFromRouteMutex.Unlock()
	fake.RemoveDestinationFromRouteStub = stub
}

func (fake *CFRouteRepository) RemoveDestinationFromRouteArgsForCall(i int) (context.Context, authorization.Info, repositories.RemoveDestinationMessage) {
	fake.removeDestinationFromRouteMutex.RLock()
	defer fake.removeDestinationFromRouteMutex.RUnlock()
	argsForCall := fake.removeDestinationFromRouteArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFRouteRepository) RemoveDestinationFromRouteReturns(result1 repositories.RouteRecord, result2 error) {
	fake.removeDestinationFromRouteMutex.Lock()
	defer fake.removeDestinationFromRouteMutex.Unlock()
	fake.RemoveDestinationFromRouteStub = nil
	fake.removeDestinationFromRouteReturns = struct {
		result1 repositories.RouteRecord
		result2 error
	}{result1, result2}
}

func (fake *CFRouteRepository) RemoveDestinationFromRouteReturnsOnCall(i int, result1 repositories.RouteRecord, result2 error) {
	fake.removeDestinationFromRouteMutex.Lock()
	defer fake.removeDestinationFromRouteMutex.Unlock()
	fake.RemoveDestinationFromRouteStub = nil
	if fake.removeDestinationFromRouteReturnsOnCall == nil {
		fake.removeDestinationFromRouteReturnsOnCall = make(map[int]struct {
			result1 repositories.RouteRecord
			result2 error
		})
	}
	fake.removeDestinationFromRouteReturnsOnCall[i] = struct {
		result1 repositories.RouteRecord
		result2 error
	}{result1, result2}
}

func (fake *CFRouteRepository) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.addDestinationsToRouteMutex.RLock()
	defer fake.addDestinationsToRouteMutex.RUnlock()
	fake.createRouteMutex.RLock()
	defer fake.createRouteMutex.RUnlock()
	fake.deleteRouteMutex.RLock()
	defer fake.deleteRouteMutex.RUnlock()
	fake.deleteUnmappedRoutesMutex.RLock()
	defer fake.deleteUnmappedRoutesMutex.RUnlock()
	fake.getRouteMutex.RLock()
	defer fake.getRouteMutex.RUnlock()
	fake.listRoutesMutex.RLock()
	defer fake.listRoutesMutex.RUnlock()
	fake.listRoutesForAppMutex.RLock()
	defer fake.listRoutesForAppMutex.RUnlock()
	fake.patchRouteMetadataMutex.RLock()
	defer fake.patchRouteMetadataMutex.RUnlock()
	fake.removeDestinationFromRouteMutex.RLock()
	defer fake.removeDestinationFromRouteMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *CFRouteRepository) recordInvocation(key string, args []interface{}) {
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

var _ handlers.CFRouteRepository = new(CFRouteRepository)
