// Code generated by MockGen. DO NOT EDIT.
// Source: pkg/tc/holder/session_holder.go

// Package mock_holder is a generated GoMock package.
package mock_holder

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	apis "github.com/opentrx/seata-golang/v2/pkg/apis"
	model "github.com/opentrx/seata-golang/v2/pkg/tc/model"
)

// MockSessionHolderInterface is a mock of SessionHolderInterface interface.
type MockSessionHolderInterface struct {
	ctrl     *gomock.Controller
	recorder *MockSessionHolderInterfaceMockRecorder
}

// MockSessionHolderInterfaceMockRecorder is the mock recorder for MockSessionHolderInterface.
type MockSessionHolderInterfaceMockRecorder struct {
	mock *MockSessionHolderInterface
}

// NewMockSessionHolderInterface creates a new mock instance.
func NewMockSessionHolderInterface(ctrl *gomock.Controller) *MockSessionHolderInterface {
	mock := &MockSessionHolderInterface{ctrl: ctrl}
	mock.recorder = &MockSessionHolderInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSessionHolderInterface) EXPECT() *MockSessionHolderInterfaceMockRecorder {
	return m.recorder
}

// AddGlobalSession mocks base method.
func (m *MockSessionHolderInterface) AddGlobalSession(session *apis.GlobalSession) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddGlobalSession", session)
	ret0, _ := ret[0].(error)
	return ret0
}

// AddGlobalSession indicates an expected call of AddGlobalSession.
func (mr *MockSessionHolderInterfaceMockRecorder) AddGlobalSession(session interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddGlobalSession", reflect.TypeOf((*MockSessionHolderInterface)(nil).AddGlobalSession), session)
}

// FindGlobalSession mocks base method.
func (m *MockSessionHolderInterface) FindGlobalSession(xid string) *apis.GlobalSession {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindGlobalSession", xid)
	ret0, _ := ret[0].(*apis.GlobalSession)
	return ret0
}

// FindGlobalSession indicates an expected call of FindGlobalSession.
func (mr *MockSessionHolderInterfaceMockRecorder) FindGlobalSession(xid interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindGlobalSession", reflect.TypeOf((*MockSessionHolderInterface)(nil).FindGlobalSession), xid)
}

// FindGlobalTransaction mocks base method.
func (m *MockSessionHolderInterface) FindGlobalTransaction(xid string) *model.GlobalTransaction {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindGlobalTransaction", xid)
	ret0, _ := ret[0].(*model.GlobalTransaction)
	return ret0
}

// FindGlobalTransaction indicates an expected call of FindGlobalTransaction.
func (mr *MockSessionHolderInterfaceMockRecorder) FindGlobalTransaction(xid interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindGlobalTransaction", reflect.TypeOf((*MockSessionHolderInterface)(nil).FindGlobalTransaction), xid)
}

// FindAsyncCommittingGlobalTransactions mocks base method.
func (m *MockSessionHolderInterface) FindAsyncCommittingGlobalTransactions(addressingIdentities []string) []*model.GlobalTransaction {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindAsyncCommittingGlobalTransactions", addressingIdentities)
	ret0, _ := ret[0].([]*model.GlobalTransaction)
	return ret0
}

// FindAsyncCommittingGlobalTransactions indicates an expected call of FindAsyncCommittingGlobalTransactions.
func (mr *MockSessionHolderInterfaceMockRecorder) FindAsyncCommittingGlobalTransactions(addressingIdentities interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindAsyncCommittingGlobalTransactions", reflect.TypeOf((*MockSessionHolderInterface)(nil).FindAsyncCommittingGlobalTransactions), addressingIdentities)
}

// FindRetryCommittingGlobalTransactions mocks base method.
func (m *MockSessionHolderInterface) FindRetryCommittingGlobalTransactions(addressingIdentities []string) []*model.GlobalTransaction {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindRetryCommittingGlobalTransactions", addressingIdentities)
	ret0, _ := ret[0].([]*model.GlobalTransaction)
	return ret0
}

// FindRetryCommittingGlobalTransactions indicates an expected call of FindRetryCommittingGlobalTransactions.
func (mr *MockSessionHolderInterfaceMockRecorder) FindRetryCommittingGlobalTransactions(addressingIdentities interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindRetryCommittingGlobalTransactions", reflect.TypeOf((*MockSessionHolderInterface)(nil).FindRetryCommittingGlobalTransactions), addressingIdentities)
}

// FindRetryRollbackGlobalTransactions mocks base method.
func (m *MockSessionHolderInterface) FindRetryRollbackGlobalTransactions(addressingIdentities []string) []*model.GlobalTransaction {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindRetryRollbackGlobalTransactions", addressingIdentities)
	ret0, _ := ret[0].([]*model.GlobalTransaction)
	return ret0
}

// FindRetryRollbackGlobalTransactions indicates an expected call of FindRetryRollbackGlobalTransactions.
func (mr *MockSessionHolderInterfaceMockRecorder) FindRetryRollbackGlobalTransactions(addressingIdentities interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindRetryRollbackGlobalTransactions", reflect.TypeOf((*MockSessionHolderInterface)(nil).FindRetryRollbackGlobalTransactions), addressingIdentities)
}

// FindGlobalSessions mocks base method.
func (m *MockSessionHolderInterface) FindGlobalSessions(statuses []apis.GlobalSession_GlobalStatus) []*apis.GlobalSession {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindGlobalSessions", statuses)
	ret0, _ := ret[0].([]*apis.GlobalSession)
	return ret0
}

// FindGlobalSessions indicates an expected call of FindGlobalSessions.
func (mr *MockSessionHolderInterfaceMockRecorder) FindGlobalSessions(statuses interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindGlobalSessions", reflect.TypeOf((*MockSessionHolderInterface)(nil).FindGlobalSessions), statuses)
}

// AllSessions mocks base method.
func (m *MockSessionHolderInterface) AllSessions() []*apis.GlobalSession {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AllSessions")
	ret0, _ := ret[0].([]*apis.GlobalSession)
	return ret0
}

// AllSessions indicates an expected call of AllSessions.
func (mr *MockSessionHolderInterfaceMockRecorder) AllSessions() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AllSessions", reflect.TypeOf((*MockSessionHolderInterface)(nil).AllSessions))
}

// UpdateGlobalSessionStatus mocks base method.
func (m *MockSessionHolderInterface) UpdateGlobalSessionStatus(session *apis.GlobalSession, status apis.GlobalSession_GlobalStatus) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateGlobalSessionStatus", session, status)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateGlobalSessionStatus indicates an expected call of UpdateGlobalSessionStatus.
func (mr *MockSessionHolderInterfaceMockRecorder) UpdateGlobalSessionStatus(session, status interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateGlobalSessionStatus", reflect.TypeOf((*MockSessionHolderInterface)(nil).UpdateGlobalSessionStatus), session, status)
}

// InactiveGlobalSession mocks base method.
func (m *MockSessionHolderInterface) InactiveGlobalSession(session *apis.GlobalSession) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InactiveGlobalSession", session)
	ret0, _ := ret[0].(error)
	return ret0
}

// InactiveGlobalSession indicates an expected call of InactiveGlobalSession.
func (mr *MockSessionHolderInterfaceMockRecorder) InactiveGlobalSession(session interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InactiveGlobalSession", reflect.TypeOf((*MockSessionHolderInterface)(nil).InactiveGlobalSession), session)
}

// RemoveGlobalSession mocks base method.
func (m *MockSessionHolderInterface) RemoveGlobalSession(session *apis.GlobalSession) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RemoveGlobalSession", session)
	ret0, _ := ret[0].(error)
	return ret0
}

// RemoveGlobalSession indicates an expected call of RemoveGlobalSession.
func (mr *MockSessionHolderInterfaceMockRecorder) RemoveGlobalSession(session interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemoveGlobalSession", reflect.TypeOf((*MockSessionHolderInterface)(nil).RemoveGlobalSession), session)
}

// RemoveGlobalTransaction mocks base method.
func (m *MockSessionHolderInterface) RemoveGlobalTransaction(globalTransaction *model.GlobalTransaction) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RemoveGlobalTransaction", globalTransaction)
	ret0, _ := ret[0].(error)
	return ret0
}

// RemoveGlobalTransaction indicates an expected call of RemoveGlobalTransaction.
func (mr *MockSessionHolderInterfaceMockRecorder) RemoveGlobalTransaction(globalTransaction interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemoveGlobalTransaction", reflect.TypeOf((*MockSessionHolderInterface)(nil).RemoveGlobalTransaction), globalTransaction)
}

// AddBranchSession mocks base method.
func (m *MockSessionHolderInterface) AddBranchSession(globalSession *apis.GlobalSession, session *apis.BranchSession) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddBranchSession", globalSession, session)
	ret0, _ := ret[0].(error)
	return ret0
}

// AddBranchSession indicates an expected call of AddBranchSession.
func (mr *MockSessionHolderInterfaceMockRecorder) AddBranchSession(globalSession, session interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddBranchSession", reflect.TypeOf((*MockSessionHolderInterface)(nil).AddBranchSession), globalSession, session)
}

// FindBranchSession mocks base method.
func (m *MockSessionHolderInterface) FindBranchSession(xid string) []*apis.BranchSession {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindBranchSession", xid)
	ret0, _ := ret[0].([]*apis.BranchSession)
	return ret0
}

// FindBranchSession indicates an expected call of FindBranchSession.
func (mr *MockSessionHolderInterfaceMockRecorder) FindBranchSession(xid interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindBranchSession", reflect.TypeOf((*MockSessionHolderInterface)(nil).FindBranchSession), xid)
}

// UpdateBranchSessionStatus mocks base method.
func (m *MockSessionHolderInterface) UpdateBranchSessionStatus(session *apis.BranchSession, status apis.BranchSession_BranchStatus) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateBranchSessionStatus", session, status)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateBranchSessionStatus indicates an expected call of UpdateBranchSessionStatus.
func (mr *MockSessionHolderInterfaceMockRecorder) UpdateBranchSessionStatus(session, status interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateBranchSessionStatus", reflect.TypeOf((*MockSessionHolderInterface)(nil).UpdateBranchSessionStatus), session, status)
}

// RemoveBranchSession mocks base method.
func (m *MockSessionHolderInterface) RemoveBranchSession(globalSession *apis.GlobalSession, session *apis.BranchSession) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RemoveBranchSession", globalSession, session)
	ret0, _ := ret[0].(error)
	return ret0
}

// RemoveBranchSession indicates an expected call of RemoveBranchSession.
func (mr *MockSessionHolderInterfaceMockRecorder) RemoveBranchSession(globalSession, session interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemoveBranchSession", reflect.TypeOf((*MockSessionHolderInterface)(nil).RemoveBranchSession), globalSession, session)
}

// findGlobalTransactions mocks base method.
func (m *MockSessionHolderInterface) findGlobalTransactions(statuses []apis.GlobalSession_GlobalStatus) []*model.GlobalTransaction {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "findGlobalTransactions", statuses)
	ret0, _ := ret[0].([]*model.GlobalTransaction)
	return ret0
}

// findGlobalTransactions indicates an expected call of findGlobalTransactions.
func (mr *MockSessionHolderInterfaceMockRecorder) findGlobalTransactions(statuses interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "findGlobalTransactions", reflect.TypeOf((*MockSessionHolderInterface)(nil).findGlobalTransactions), statuses)
}

// findGlobalTransactionsWithAddressingIdentities mocks base method.
func (m *MockSessionHolderInterface) findGlobalTransactionsWithAddressingIdentities(statuses []apis.GlobalSession_GlobalStatus, addressingIdentities []string) []*model.GlobalTransaction {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "findGlobalTransactionsWithAddressingIdentities", statuses, addressingIdentities)
	ret0, _ := ret[0].([]*model.GlobalTransaction)
	return ret0
}

// findGlobalTransactionsWithAddressingIdentities indicates an expected call of findGlobalTransactionsWithAddressingIdentities.
func (mr *MockSessionHolderInterfaceMockRecorder) findGlobalTransactionsWithAddressingIdentities(statuses, addressingIdentities interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "findGlobalTransactionsWithAddressingIdentities", reflect.TypeOf((*MockSessionHolderInterface)(nil).findGlobalTransactionsWithAddressingIdentities), statuses, addressingIdentities)
}

// findGlobalTransactionsByGlobalSessions mocks base method.
func (m *MockSessionHolderInterface) findGlobalTransactionsByGlobalSessions(sessions []*apis.GlobalSession) []*model.GlobalTransaction {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "findGlobalTransactionsByGlobalSessions", sessions)
	ret0, _ := ret[0].([]*model.GlobalTransaction)
	return ret0
}

// findGlobalTransactionsByGlobalSessions indicates an expected call of findGlobalTransactionsByGlobalSessions.
func (mr *MockSessionHolderInterfaceMockRecorder) findGlobalTransactionsByGlobalSessions(sessions interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "findGlobalTransactionsByGlobalSessions", reflect.TypeOf((*MockSessionHolderInterface)(nil).findGlobalTransactionsByGlobalSessions), sessions)
}
