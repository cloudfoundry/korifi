// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services"
)

type VCAPServicesSecretBuilder struct {
	BuildVCAPServicesEnvValueStub        func(context.Context, *v1alpha1.CFApp) (string, error)
	buildVCAPServicesEnvValueMutex       sync.RWMutex
	buildVCAPServicesEnvValueArgsForCall []struct {
		arg1 context.Context
		arg2 *v1alpha1.CFApp
	}
	buildVCAPServicesEnvValueReturns struct {
		result1 string
		result2 error
	}
	buildVCAPServicesEnvValueReturnsOnCall map[int]struct {
		result1 string
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *VCAPServicesSecretBuilder) BuildVCAPServicesEnvValue(arg1 context.Context, arg2 *v1alpha1.CFApp) (string, error) {
	fake.buildVCAPServicesEnvValueMutex.Lock()
	ret, specificReturn := fake.buildVCAPServicesEnvValueReturnsOnCall[len(fake.buildVCAPServicesEnvValueArgsForCall)]
	fake.buildVCAPServicesEnvValueArgsForCall = append(fake.buildVCAPServicesEnvValueArgsForCall, struct {
		arg1 context.Context
		arg2 *v1alpha1.CFApp
	}{arg1, arg2})
	stub := fake.BuildVCAPServicesEnvValueStub
	fakeReturns := fake.buildVCAPServicesEnvValueReturns
	fake.recordInvocation("BuildVCAPServicesEnvValue", []interface{}{arg1, arg2})
	fake.buildVCAPServicesEnvValueMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *VCAPServicesSecretBuilder) BuildVCAPServicesEnvValueCallCount() int {
	fake.buildVCAPServicesEnvValueMutex.RLock()
	defer fake.buildVCAPServicesEnvValueMutex.RUnlock()
	return len(fake.buildVCAPServicesEnvValueArgsForCall)
}

func (fake *VCAPServicesSecretBuilder) BuildVCAPServicesEnvValueCalls(stub func(context.Context, *v1alpha1.CFApp) (string, error)) {
	fake.buildVCAPServicesEnvValueMutex.Lock()
	defer fake.buildVCAPServicesEnvValueMutex.Unlock()
	fake.BuildVCAPServicesEnvValueStub = stub
}

func (fake *VCAPServicesSecretBuilder) BuildVCAPServicesEnvValueArgsForCall(i int) (context.Context, *v1alpha1.CFApp) {
	fake.buildVCAPServicesEnvValueMutex.RLock()
	defer fake.buildVCAPServicesEnvValueMutex.RUnlock()
	argsForCall := fake.buildVCAPServicesEnvValueArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2
}

func (fake *VCAPServicesSecretBuilder) BuildVCAPServicesEnvValueReturns(result1 string, result2 error) {
	fake.buildVCAPServicesEnvValueMutex.Lock()
	defer fake.buildVCAPServicesEnvValueMutex.Unlock()
	fake.BuildVCAPServicesEnvValueStub = nil
	fake.buildVCAPServicesEnvValueReturns = struct {
		result1 string
		result2 error
	}{result1, result2}
}

func (fake *VCAPServicesSecretBuilder) BuildVCAPServicesEnvValueReturnsOnCall(i int, result1 string, result2 error) {
	fake.buildVCAPServicesEnvValueMutex.Lock()
	defer fake.buildVCAPServicesEnvValueMutex.Unlock()
	fake.BuildVCAPServicesEnvValueStub = nil
	if fake.buildVCAPServicesEnvValueReturnsOnCall == nil {
		fake.buildVCAPServicesEnvValueReturnsOnCall = make(map[int]struct {
			result1 string
			result2 error
		})
	}
	fake.buildVCAPServicesEnvValueReturnsOnCall[i] = struct {
		result1 string
		result2 error
	}{result1, result2}
}

func (fake *VCAPServicesSecretBuilder) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.buildVCAPServicesEnvValueMutex.RLock()
	defer fake.buildVCAPServicesEnvValueMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *VCAPServicesSecretBuilder) recordInvocation(key string, args []interface{}) {
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

var _ services.VCAPServicesSecretBuilder = new(VCAPServicesSecretBuilder)
