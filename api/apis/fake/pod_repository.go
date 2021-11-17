// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PodRepository struct {
	FetchPodStatsByAppGUIDStub        func(context.Context, client.Client, repositories.FetchPodStatsMessage) ([]repositories.PodStatsRecord, error)
	fetchPodStatsByAppGUIDMutex       sync.RWMutex
	fetchPodStatsByAppGUIDArgsForCall []struct {
		arg1 context.Context
		arg2 client.Client
		arg3 repositories.FetchPodStatsMessage
	}
	fetchPodStatsByAppGUIDReturns struct {
		result1 []repositories.PodStatsRecord
		result2 error
	}
	fetchPodStatsByAppGUIDReturnsOnCall map[int]struct {
		result1 []repositories.PodStatsRecord
		result2 error
	}
	WatchForPodsTerminationStub        func(context.Context, client.Client, string, string) (bool, error)
	watchForPodsTerminationMutex       sync.RWMutex
	watchForPodsTerminationArgsForCall []struct {
		arg1 context.Context
		arg2 client.Client
		arg3 string
		arg4 string
	}
	watchForPodsTerminationReturns struct {
		result1 bool
		result2 error
	}
	watchForPodsTerminationReturnsOnCall map[int]struct {
		result1 bool
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *PodRepository) FetchPodStatsByAppGUID(arg1 context.Context, arg2 client.Client, arg3 repositories.FetchPodStatsMessage) ([]repositories.PodStatsRecord, error) {
	fake.fetchPodStatsByAppGUIDMutex.Lock()
	ret, specificReturn := fake.fetchPodStatsByAppGUIDReturnsOnCall[len(fake.fetchPodStatsByAppGUIDArgsForCall)]
	fake.fetchPodStatsByAppGUIDArgsForCall = append(fake.fetchPodStatsByAppGUIDArgsForCall, struct {
		arg1 context.Context
		arg2 client.Client
		arg3 repositories.FetchPodStatsMessage
	}{arg1, arg2, arg3})
	stub := fake.FetchPodStatsByAppGUIDStub
	fakeReturns := fake.fetchPodStatsByAppGUIDReturns
	fake.recordInvocation("FetchPodStatsByAppGUID", []interface{}{arg1, arg2, arg3})
	fake.fetchPodStatsByAppGUIDMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *PodRepository) FetchPodStatsByAppGUIDCallCount() int {
	fake.fetchPodStatsByAppGUIDMutex.RLock()
	defer fake.fetchPodStatsByAppGUIDMutex.RUnlock()
	return len(fake.fetchPodStatsByAppGUIDArgsForCall)
}

func (fake *PodRepository) FetchPodStatsByAppGUIDCalls(stub func(context.Context, client.Client, repositories.FetchPodStatsMessage) ([]repositories.PodStatsRecord, error)) {
	fake.fetchPodStatsByAppGUIDMutex.Lock()
	defer fake.fetchPodStatsByAppGUIDMutex.Unlock()
	fake.FetchPodStatsByAppGUIDStub = stub
}

func (fake *PodRepository) FetchPodStatsByAppGUIDArgsForCall(i int) (context.Context, client.Client, repositories.FetchPodStatsMessage) {
	fake.fetchPodStatsByAppGUIDMutex.RLock()
	defer fake.fetchPodStatsByAppGUIDMutex.RUnlock()
	argsForCall := fake.fetchPodStatsByAppGUIDArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *PodRepository) FetchPodStatsByAppGUIDReturns(result1 []repositories.PodStatsRecord, result2 error) {
	fake.fetchPodStatsByAppGUIDMutex.Lock()
	defer fake.fetchPodStatsByAppGUIDMutex.Unlock()
	fake.FetchPodStatsByAppGUIDStub = nil
	fake.fetchPodStatsByAppGUIDReturns = struct {
		result1 []repositories.PodStatsRecord
		result2 error
	}{result1, result2}
}

func (fake *PodRepository) FetchPodStatsByAppGUIDReturnsOnCall(i int, result1 []repositories.PodStatsRecord, result2 error) {
	fake.fetchPodStatsByAppGUIDMutex.Lock()
	defer fake.fetchPodStatsByAppGUIDMutex.Unlock()
	fake.FetchPodStatsByAppGUIDStub = nil
	if fake.fetchPodStatsByAppGUIDReturnsOnCall == nil {
		fake.fetchPodStatsByAppGUIDReturnsOnCall = make(map[int]struct {
			result1 []repositories.PodStatsRecord
			result2 error
		})
	}
	fake.fetchPodStatsByAppGUIDReturnsOnCall[i] = struct {
		result1 []repositories.PodStatsRecord
		result2 error
	}{result1, result2}
}

func (fake *PodRepository) WatchForPodsTermination(arg1 context.Context, arg2 client.Client, arg3 string, arg4 string) (bool, error) {
	fake.watchForPodsTerminationMutex.Lock()
	ret, specificReturn := fake.watchForPodsTerminationReturnsOnCall[len(fake.watchForPodsTerminationArgsForCall)]
	fake.watchForPodsTerminationArgsForCall = append(fake.watchForPodsTerminationArgsForCall, struct {
		arg1 context.Context
		arg2 client.Client
		arg3 string
		arg4 string
	}{arg1, arg2, arg3, arg4})
	stub := fake.WatchForPodsTerminationStub
	fakeReturns := fake.watchForPodsTerminationReturns
	fake.recordInvocation("WatchForPodsTermination", []interface{}{arg1, arg2, arg3, arg4})
	fake.watchForPodsTerminationMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3, arg4)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *PodRepository) WatchForPodsTerminationCallCount() int {
	fake.watchForPodsTerminationMutex.RLock()
	defer fake.watchForPodsTerminationMutex.RUnlock()
	return len(fake.watchForPodsTerminationArgsForCall)
}

func (fake *PodRepository) WatchForPodsTerminationCalls(stub func(context.Context, client.Client, string, string) (bool, error)) {
	fake.watchForPodsTerminationMutex.Lock()
	defer fake.watchForPodsTerminationMutex.Unlock()
	fake.WatchForPodsTerminationStub = stub
}

func (fake *PodRepository) WatchForPodsTerminationArgsForCall(i int) (context.Context, client.Client, string, string) {
	fake.watchForPodsTerminationMutex.RLock()
	defer fake.watchForPodsTerminationMutex.RUnlock()
	argsForCall := fake.watchForPodsTerminationArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3, argsForCall.arg4
}

func (fake *PodRepository) WatchForPodsTerminationReturns(result1 bool, result2 error) {
	fake.watchForPodsTerminationMutex.Lock()
	defer fake.watchForPodsTerminationMutex.Unlock()
	fake.WatchForPodsTerminationStub = nil
	fake.watchForPodsTerminationReturns = struct {
		result1 bool
		result2 error
	}{result1, result2}
}

func (fake *PodRepository) WatchForPodsTerminationReturnsOnCall(i int, result1 bool, result2 error) {
	fake.watchForPodsTerminationMutex.Lock()
	defer fake.watchForPodsTerminationMutex.Unlock()
	fake.WatchForPodsTerminationStub = nil
	if fake.watchForPodsTerminationReturnsOnCall == nil {
		fake.watchForPodsTerminationReturnsOnCall = make(map[int]struct {
			result1 bool
			result2 error
		})
	}
	fake.watchForPodsTerminationReturnsOnCall[i] = struct {
		result1 bool
		result2 error
	}{result1, result2}
}

func (fake *PodRepository) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.fetchPodStatsByAppGUIDMutex.RLock()
	defer fake.fetchPodStatsByAppGUIDMutex.RUnlock()
	fake.watchForPodsTerminationMutex.RLock()
	defer fake.watchForPodsTerminationMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *PodRepository) recordInvocation(key string, args []interface{}) {
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

var _ apis.PodRepository = new(PodRepository)
