// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type CFDeploymentRepository struct {
	CreateDeploymentStub        func(context.Context, authorization.Info, repositories.CreateDeploymentMessage) (repositories.DeploymentRecord, error)
	createDeploymentMutex       sync.RWMutex
	createDeploymentArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.CreateDeploymentMessage
	}
	createDeploymentReturns struct {
		result1 repositories.DeploymentRecord
		result2 error
	}
	createDeploymentReturnsOnCall map[int]struct {
		result1 repositories.DeploymentRecord
		result2 error
	}
	GetDeploymentStub        func(context.Context, authorization.Info, string) (repositories.DeploymentRecord, error)
	getDeploymentMutex       sync.RWMutex
	getDeploymentArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
	}
	getDeploymentReturns struct {
		result1 repositories.DeploymentRecord
		result2 error
	}
	getDeploymentReturnsOnCall map[int]struct {
		result1 repositories.DeploymentRecord
		result2 error
	}
	ListDeploymentsStub        func(context.Context, authorization.Info, repositories.ListDeploymentsMessage) (repositories.ListResult[repositories.DeploymentRecord], error)
	listDeploymentsMutex       sync.RWMutex
	listDeploymentsArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListDeploymentsMessage
	}
	listDeploymentsReturns struct {
		result1 repositories.ListResult[repositories.DeploymentRecord]
		result2 error
	}
	listDeploymentsReturnsOnCall map[int]struct {
		result1 repositories.ListResult[repositories.DeploymentRecord]
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *CFDeploymentRepository) CreateDeployment(arg1 context.Context, arg2 authorization.Info, arg3 repositories.CreateDeploymentMessage) (repositories.DeploymentRecord, error) {
	fake.createDeploymentMutex.Lock()
	ret, specificReturn := fake.createDeploymentReturnsOnCall[len(fake.createDeploymentArgsForCall)]
	fake.createDeploymentArgsForCall = append(fake.createDeploymentArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.CreateDeploymentMessage
	}{arg1, arg2, arg3})
	stub := fake.CreateDeploymentStub
	fakeReturns := fake.createDeploymentReturns
	fake.recordInvocation("CreateDeployment", []interface{}{arg1, arg2, arg3})
	fake.createDeploymentMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *CFDeploymentRepository) CreateDeploymentCallCount() int {
	fake.createDeploymentMutex.RLock()
	defer fake.createDeploymentMutex.RUnlock()
	return len(fake.createDeploymentArgsForCall)
}

func (fake *CFDeploymentRepository) CreateDeploymentCalls(stub func(context.Context, authorization.Info, repositories.CreateDeploymentMessage) (repositories.DeploymentRecord, error)) {
	fake.createDeploymentMutex.Lock()
	defer fake.createDeploymentMutex.Unlock()
	fake.CreateDeploymentStub = stub
}

func (fake *CFDeploymentRepository) CreateDeploymentArgsForCall(i int) (context.Context, authorization.Info, repositories.CreateDeploymentMessage) {
	fake.createDeploymentMutex.RLock()
	defer fake.createDeploymentMutex.RUnlock()
	argsForCall := fake.createDeploymentArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFDeploymentRepository) CreateDeploymentReturns(result1 repositories.DeploymentRecord, result2 error) {
	fake.createDeploymentMutex.Lock()
	defer fake.createDeploymentMutex.Unlock()
	fake.CreateDeploymentStub = nil
	fake.createDeploymentReturns = struct {
		result1 repositories.DeploymentRecord
		result2 error
	}{result1, result2}
}

func (fake *CFDeploymentRepository) CreateDeploymentReturnsOnCall(i int, result1 repositories.DeploymentRecord, result2 error) {
	fake.createDeploymentMutex.Lock()
	defer fake.createDeploymentMutex.Unlock()
	fake.CreateDeploymentStub = nil
	if fake.createDeploymentReturnsOnCall == nil {
		fake.createDeploymentReturnsOnCall = make(map[int]struct {
			result1 repositories.DeploymentRecord
			result2 error
		})
	}
	fake.createDeploymentReturnsOnCall[i] = struct {
		result1 repositories.DeploymentRecord
		result2 error
	}{result1, result2}
}

func (fake *CFDeploymentRepository) GetDeployment(arg1 context.Context, arg2 authorization.Info, arg3 string) (repositories.DeploymentRecord, error) {
	fake.getDeploymentMutex.Lock()
	ret, specificReturn := fake.getDeploymentReturnsOnCall[len(fake.getDeploymentArgsForCall)]
	fake.getDeploymentArgsForCall = append(fake.getDeploymentArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
	}{arg1, arg2, arg3})
	stub := fake.GetDeploymentStub
	fakeReturns := fake.getDeploymentReturns
	fake.recordInvocation("GetDeployment", []interface{}{arg1, arg2, arg3})
	fake.getDeploymentMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *CFDeploymentRepository) GetDeploymentCallCount() int {
	fake.getDeploymentMutex.RLock()
	defer fake.getDeploymentMutex.RUnlock()
	return len(fake.getDeploymentArgsForCall)
}

func (fake *CFDeploymentRepository) GetDeploymentCalls(stub func(context.Context, authorization.Info, string) (repositories.DeploymentRecord, error)) {
	fake.getDeploymentMutex.Lock()
	defer fake.getDeploymentMutex.Unlock()
	fake.GetDeploymentStub = stub
}

func (fake *CFDeploymentRepository) GetDeploymentArgsForCall(i int) (context.Context, authorization.Info, string) {
	fake.getDeploymentMutex.RLock()
	defer fake.getDeploymentMutex.RUnlock()
	argsForCall := fake.getDeploymentArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFDeploymentRepository) GetDeploymentReturns(result1 repositories.DeploymentRecord, result2 error) {
	fake.getDeploymentMutex.Lock()
	defer fake.getDeploymentMutex.Unlock()
	fake.GetDeploymentStub = nil
	fake.getDeploymentReturns = struct {
		result1 repositories.DeploymentRecord
		result2 error
	}{result1, result2}
}

func (fake *CFDeploymentRepository) GetDeploymentReturnsOnCall(i int, result1 repositories.DeploymentRecord, result2 error) {
	fake.getDeploymentMutex.Lock()
	defer fake.getDeploymentMutex.Unlock()
	fake.GetDeploymentStub = nil
	if fake.getDeploymentReturnsOnCall == nil {
		fake.getDeploymentReturnsOnCall = make(map[int]struct {
			result1 repositories.DeploymentRecord
			result2 error
		})
	}
	fake.getDeploymentReturnsOnCall[i] = struct {
		result1 repositories.DeploymentRecord
		result2 error
	}{result1, result2}
}

func (fake *CFDeploymentRepository) ListDeployments(arg1 context.Context, arg2 authorization.Info, arg3 repositories.ListDeploymentsMessage) (repositories.ListResult[repositories.DeploymentRecord], error) {
	fake.listDeploymentsMutex.Lock()
	ret, specificReturn := fake.listDeploymentsReturnsOnCall[len(fake.listDeploymentsArgsForCall)]
	fake.listDeploymentsArgsForCall = append(fake.listDeploymentsArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListDeploymentsMessage
	}{arg1, arg2, arg3})
	stub := fake.ListDeploymentsStub
	fakeReturns := fake.listDeploymentsReturns
	fake.recordInvocation("ListDeployments", []interface{}{arg1, arg2, arg3})
	fake.listDeploymentsMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *CFDeploymentRepository) ListDeploymentsCallCount() int {
	fake.listDeploymentsMutex.RLock()
	defer fake.listDeploymentsMutex.RUnlock()
	return len(fake.listDeploymentsArgsForCall)
}

func (fake *CFDeploymentRepository) ListDeploymentsCalls(stub func(context.Context, authorization.Info, repositories.ListDeploymentsMessage) (repositories.ListResult[repositories.DeploymentRecord], error)) {
	fake.listDeploymentsMutex.Lock()
	defer fake.listDeploymentsMutex.Unlock()
	fake.ListDeploymentsStub = stub
}

func (fake *CFDeploymentRepository) ListDeploymentsArgsForCall(i int) (context.Context, authorization.Info, repositories.ListDeploymentsMessage) {
	fake.listDeploymentsMutex.RLock()
	defer fake.listDeploymentsMutex.RUnlock()
	argsForCall := fake.listDeploymentsArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFDeploymentRepository) ListDeploymentsReturns(result1 repositories.ListResult[repositories.DeploymentRecord], result2 error) {
	fake.listDeploymentsMutex.Lock()
	defer fake.listDeploymentsMutex.Unlock()
	fake.ListDeploymentsStub = nil
	fake.listDeploymentsReturns = struct {
		result1 repositories.ListResult[repositories.DeploymentRecord]
		result2 error
	}{result1, result2}
}

func (fake *CFDeploymentRepository) ListDeploymentsReturnsOnCall(i int, result1 repositories.ListResult[repositories.DeploymentRecord], result2 error) {
	fake.listDeploymentsMutex.Lock()
	defer fake.listDeploymentsMutex.Unlock()
	fake.ListDeploymentsStub = nil
	if fake.listDeploymentsReturnsOnCall == nil {
		fake.listDeploymentsReturnsOnCall = make(map[int]struct {
			result1 repositories.ListResult[repositories.DeploymentRecord]
			result2 error
		})
	}
	fake.listDeploymentsReturnsOnCall[i] = struct {
		result1 repositories.ListResult[repositories.DeploymentRecord]
		result2 error
	}{result1, result2}
}

func (fake *CFDeploymentRepository) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *CFDeploymentRepository) recordInvocation(key string, args []interface{}) {
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

var _ handlers.CFDeploymentRepository = new(CFDeploymentRepository)
