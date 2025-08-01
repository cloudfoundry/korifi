// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type CFServiceOfferingRepository struct {
	DeleteOfferingStub        func(context.Context, authorization.Info, repositories.DeleteServiceOfferingMessage) error
	deleteOfferingMutex       sync.RWMutex
	deleteOfferingArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.DeleteServiceOfferingMessage
	}
	deleteOfferingReturns struct {
		result1 error
	}
	deleteOfferingReturnsOnCall map[int]struct {
		result1 error
	}
	GetServiceOfferingStub        func(context.Context, authorization.Info, string) (repositories.ServiceOfferingRecord, error)
	getServiceOfferingMutex       sync.RWMutex
	getServiceOfferingArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
	}
	getServiceOfferingReturns struct {
		result1 repositories.ServiceOfferingRecord
		result2 error
	}
	getServiceOfferingReturnsOnCall map[int]struct {
		result1 repositories.ServiceOfferingRecord
		result2 error
	}
	ListOfferingsStub        func(context.Context, authorization.Info, repositories.ListServiceOfferingMessage) (repositories.ListResult[repositories.ServiceOfferingRecord], error)
	listOfferingsMutex       sync.RWMutex
	listOfferingsArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListServiceOfferingMessage
	}
	listOfferingsReturns struct {
		result1 repositories.ListResult[repositories.ServiceOfferingRecord]
		result2 error
	}
	listOfferingsReturnsOnCall map[int]struct {
		result1 repositories.ListResult[repositories.ServiceOfferingRecord]
		result2 error
	}
	UpdateServiceOfferingStub        func(context.Context, authorization.Info, repositories.UpdateServiceOfferingMessage) (repositories.ServiceOfferingRecord, error)
	updateServiceOfferingMutex       sync.RWMutex
	updateServiceOfferingArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.UpdateServiceOfferingMessage
	}
	updateServiceOfferingReturns struct {
		result1 repositories.ServiceOfferingRecord
		result2 error
	}
	updateServiceOfferingReturnsOnCall map[int]struct {
		result1 repositories.ServiceOfferingRecord
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *CFServiceOfferingRepository) DeleteOffering(arg1 context.Context, arg2 authorization.Info, arg3 repositories.DeleteServiceOfferingMessage) error {
	fake.deleteOfferingMutex.Lock()
	ret, specificReturn := fake.deleteOfferingReturnsOnCall[len(fake.deleteOfferingArgsForCall)]
	fake.deleteOfferingArgsForCall = append(fake.deleteOfferingArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.DeleteServiceOfferingMessage
	}{arg1, arg2, arg3})
	stub := fake.DeleteOfferingStub
	fakeReturns := fake.deleteOfferingReturns
	fake.recordInvocation("DeleteOffering", []interface{}{arg1, arg2, arg3})
	fake.deleteOfferingMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *CFServiceOfferingRepository) DeleteOfferingCallCount() int {
	fake.deleteOfferingMutex.RLock()
	defer fake.deleteOfferingMutex.RUnlock()
	return len(fake.deleteOfferingArgsForCall)
}

func (fake *CFServiceOfferingRepository) DeleteOfferingCalls(stub func(context.Context, authorization.Info, repositories.DeleteServiceOfferingMessage) error) {
	fake.deleteOfferingMutex.Lock()
	defer fake.deleteOfferingMutex.Unlock()
	fake.DeleteOfferingStub = stub
}

func (fake *CFServiceOfferingRepository) DeleteOfferingArgsForCall(i int) (context.Context, authorization.Info, repositories.DeleteServiceOfferingMessage) {
	fake.deleteOfferingMutex.RLock()
	defer fake.deleteOfferingMutex.RUnlock()
	argsForCall := fake.deleteOfferingArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFServiceOfferingRepository) DeleteOfferingReturns(result1 error) {
	fake.deleteOfferingMutex.Lock()
	defer fake.deleteOfferingMutex.Unlock()
	fake.DeleteOfferingStub = nil
	fake.deleteOfferingReturns = struct {
		result1 error
	}{result1}
}

func (fake *CFServiceOfferingRepository) DeleteOfferingReturnsOnCall(i int, result1 error) {
	fake.deleteOfferingMutex.Lock()
	defer fake.deleteOfferingMutex.Unlock()
	fake.DeleteOfferingStub = nil
	if fake.deleteOfferingReturnsOnCall == nil {
		fake.deleteOfferingReturnsOnCall = make(map[int]struct {
			result1 error
		})
	}
	fake.deleteOfferingReturnsOnCall[i] = struct {
		result1 error
	}{result1}
}

func (fake *CFServiceOfferingRepository) GetServiceOffering(arg1 context.Context, arg2 authorization.Info, arg3 string) (repositories.ServiceOfferingRecord, error) {
	fake.getServiceOfferingMutex.Lock()
	ret, specificReturn := fake.getServiceOfferingReturnsOnCall[len(fake.getServiceOfferingArgsForCall)]
	fake.getServiceOfferingArgsForCall = append(fake.getServiceOfferingArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
	}{arg1, arg2, arg3})
	stub := fake.GetServiceOfferingStub
	fakeReturns := fake.getServiceOfferingReturns
	fake.recordInvocation("GetServiceOffering", []interface{}{arg1, arg2, arg3})
	fake.getServiceOfferingMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *CFServiceOfferingRepository) GetServiceOfferingCallCount() int {
	fake.getServiceOfferingMutex.RLock()
	defer fake.getServiceOfferingMutex.RUnlock()
	return len(fake.getServiceOfferingArgsForCall)
}

func (fake *CFServiceOfferingRepository) GetServiceOfferingCalls(stub func(context.Context, authorization.Info, string) (repositories.ServiceOfferingRecord, error)) {
	fake.getServiceOfferingMutex.Lock()
	defer fake.getServiceOfferingMutex.Unlock()
	fake.GetServiceOfferingStub = stub
}

func (fake *CFServiceOfferingRepository) GetServiceOfferingArgsForCall(i int) (context.Context, authorization.Info, string) {
	fake.getServiceOfferingMutex.RLock()
	defer fake.getServiceOfferingMutex.RUnlock()
	argsForCall := fake.getServiceOfferingArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFServiceOfferingRepository) GetServiceOfferingReturns(result1 repositories.ServiceOfferingRecord, result2 error) {
	fake.getServiceOfferingMutex.Lock()
	defer fake.getServiceOfferingMutex.Unlock()
	fake.GetServiceOfferingStub = nil
	fake.getServiceOfferingReturns = struct {
		result1 repositories.ServiceOfferingRecord
		result2 error
	}{result1, result2}
}

func (fake *CFServiceOfferingRepository) GetServiceOfferingReturnsOnCall(i int, result1 repositories.ServiceOfferingRecord, result2 error) {
	fake.getServiceOfferingMutex.Lock()
	defer fake.getServiceOfferingMutex.Unlock()
	fake.GetServiceOfferingStub = nil
	if fake.getServiceOfferingReturnsOnCall == nil {
		fake.getServiceOfferingReturnsOnCall = make(map[int]struct {
			result1 repositories.ServiceOfferingRecord
			result2 error
		})
	}
	fake.getServiceOfferingReturnsOnCall[i] = struct {
		result1 repositories.ServiceOfferingRecord
		result2 error
	}{result1, result2}
}

func (fake *CFServiceOfferingRepository) ListOfferings(arg1 context.Context, arg2 authorization.Info, arg3 repositories.ListServiceOfferingMessage) (repositories.ListResult[repositories.ServiceOfferingRecord], error) {
	fake.listOfferingsMutex.Lock()
	ret, specificReturn := fake.listOfferingsReturnsOnCall[len(fake.listOfferingsArgsForCall)]
	fake.listOfferingsArgsForCall = append(fake.listOfferingsArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListServiceOfferingMessage
	}{arg1, arg2, arg3})
	stub := fake.ListOfferingsStub
	fakeReturns := fake.listOfferingsReturns
	fake.recordInvocation("ListOfferings", []interface{}{arg1, arg2, arg3})
	fake.listOfferingsMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *CFServiceOfferingRepository) ListOfferingsCallCount() int {
	fake.listOfferingsMutex.RLock()
	defer fake.listOfferingsMutex.RUnlock()
	return len(fake.listOfferingsArgsForCall)
}

func (fake *CFServiceOfferingRepository) ListOfferingsCalls(stub func(context.Context, authorization.Info, repositories.ListServiceOfferingMessage) (repositories.ListResult[repositories.ServiceOfferingRecord], error)) {
	fake.listOfferingsMutex.Lock()
	defer fake.listOfferingsMutex.Unlock()
	fake.ListOfferingsStub = stub
}

func (fake *CFServiceOfferingRepository) ListOfferingsArgsForCall(i int) (context.Context, authorization.Info, repositories.ListServiceOfferingMessage) {
	fake.listOfferingsMutex.RLock()
	defer fake.listOfferingsMutex.RUnlock()
	argsForCall := fake.listOfferingsArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFServiceOfferingRepository) ListOfferingsReturns(result1 repositories.ListResult[repositories.ServiceOfferingRecord], result2 error) {
	fake.listOfferingsMutex.Lock()
	defer fake.listOfferingsMutex.Unlock()
	fake.ListOfferingsStub = nil
	fake.listOfferingsReturns = struct {
		result1 repositories.ListResult[repositories.ServiceOfferingRecord]
		result2 error
	}{result1, result2}
}

func (fake *CFServiceOfferingRepository) ListOfferingsReturnsOnCall(i int, result1 repositories.ListResult[repositories.ServiceOfferingRecord], result2 error) {
	fake.listOfferingsMutex.Lock()
	defer fake.listOfferingsMutex.Unlock()
	fake.ListOfferingsStub = nil
	if fake.listOfferingsReturnsOnCall == nil {
		fake.listOfferingsReturnsOnCall = make(map[int]struct {
			result1 repositories.ListResult[repositories.ServiceOfferingRecord]
			result2 error
		})
	}
	fake.listOfferingsReturnsOnCall[i] = struct {
		result1 repositories.ListResult[repositories.ServiceOfferingRecord]
		result2 error
	}{result1, result2}
}

func (fake *CFServiceOfferingRepository) UpdateServiceOffering(arg1 context.Context, arg2 authorization.Info, arg3 repositories.UpdateServiceOfferingMessage) (repositories.ServiceOfferingRecord, error) {
	fake.updateServiceOfferingMutex.Lock()
	ret, specificReturn := fake.updateServiceOfferingReturnsOnCall[len(fake.updateServiceOfferingArgsForCall)]
	fake.updateServiceOfferingArgsForCall = append(fake.updateServiceOfferingArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.UpdateServiceOfferingMessage
	}{arg1, arg2, arg3})
	stub := fake.UpdateServiceOfferingStub
	fakeReturns := fake.updateServiceOfferingReturns
	fake.recordInvocation("UpdateServiceOffering", []interface{}{arg1, arg2, arg3})
	fake.updateServiceOfferingMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *CFServiceOfferingRepository) UpdateServiceOfferingCallCount() int {
	fake.updateServiceOfferingMutex.RLock()
	defer fake.updateServiceOfferingMutex.RUnlock()
	return len(fake.updateServiceOfferingArgsForCall)
}

func (fake *CFServiceOfferingRepository) UpdateServiceOfferingCalls(stub func(context.Context, authorization.Info, repositories.UpdateServiceOfferingMessage) (repositories.ServiceOfferingRecord, error)) {
	fake.updateServiceOfferingMutex.Lock()
	defer fake.updateServiceOfferingMutex.Unlock()
	fake.UpdateServiceOfferingStub = stub
}

func (fake *CFServiceOfferingRepository) UpdateServiceOfferingArgsForCall(i int) (context.Context, authorization.Info, repositories.UpdateServiceOfferingMessage) {
	fake.updateServiceOfferingMutex.RLock()
	defer fake.updateServiceOfferingMutex.RUnlock()
	argsForCall := fake.updateServiceOfferingArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFServiceOfferingRepository) UpdateServiceOfferingReturns(result1 repositories.ServiceOfferingRecord, result2 error) {
	fake.updateServiceOfferingMutex.Lock()
	defer fake.updateServiceOfferingMutex.Unlock()
	fake.UpdateServiceOfferingStub = nil
	fake.updateServiceOfferingReturns = struct {
		result1 repositories.ServiceOfferingRecord
		result2 error
	}{result1, result2}
}

func (fake *CFServiceOfferingRepository) UpdateServiceOfferingReturnsOnCall(i int, result1 repositories.ServiceOfferingRecord, result2 error) {
	fake.updateServiceOfferingMutex.Lock()
	defer fake.updateServiceOfferingMutex.Unlock()
	fake.UpdateServiceOfferingStub = nil
	if fake.updateServiceOfferingReturnsOnCall == nil {
		fake.updateServiceOfferingReturnsOnCall = make(map[int]struct {
			result1 repositories.ServiceOfferingRecord
			result2 error
		})
	}
	fake.updateServiceOfferingReturnsOnCall[i] = struct {
		result1 repositories.ServiceOfferingRecord
		result2 error
	}{result1, result2}
}

func (fake *CFServiceOfferingRepository) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *CFServiceOfferingRepository) recordInvocation(key string, args []interface{}) {
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

var _ handlers.CFServiceOfferingRepository = new(CFServiceOfferingRepository)
