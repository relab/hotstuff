// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/relab/hotstuff/modules (interfaces: Configuration)

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	hotstuff "github.com/relab/hotstuff"
	modules "github.com/relab/hotstuff/modules"
	msg "github.com/relab/hotstuff/msg"
)

// MockConfiguration is a mock of Configuration interface.
type MockConfiguration struct {
	ctrl     *gomock.Controller
	recorder *MockConfigurationMockRecorder
}

// MockConfigurationMockRecorder is the mock recorder for MockConfiguration.
type MockConfigurationMockRecorder struct {
	mock *MockConfiguration
}

// NewMockConfiguration creates a new mock instance.
func NewMockConfiguration(ctrl *gomock.Controller) *MockConfiguration {
	mock := &MockConfiguration{ctrl: ctrl}
	mock.recorder = &MockConfigurationMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockConfiguration) EXPECT() *MockConfigurationMockRecorder {
	return m.recorder
}

// Fetch mocks base method.
func (m *MockConfiguration) Fetch(arg0 context.Context, arg1 msg.Hash) (*msg.Block, bool) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Fetch", arg0, arg1)
	ret0, _ := ret[0].(*msg.Block)
	ret1, _ := ret[1].(bool)
	return ret0, ret1
}

// Fetch indicates an expected call of Fetch.
func (mr *MockConfigurationMockRecorder) Fetch(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Fetch", reflect.TypeOf((*MockConfiguration)(nil).Fetch), arg0, arg1)
}

// Len mocks base method.
func (m *MockConfiguration) Len() int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Len")
	ret0, _ := ret[0].(int)
	return ret0
}

// Len indicates an expected call of Len.
func (mr *MockConfigurationMockRecorder) Len() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Len", reflect.TypeOf((*MockConfiguration)(nil).Len))
}

// Propose mocks base method.
func (m *MockConfiguration) Propose(arg0 *msg.Proposal) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Propose", arg0)
}

// Propose indicates an expected call of Propose.
func (mr *MockConfigurationMockRecorder) Propose(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Propose", reflect.TypeOf((*MockConfiguration)(nil).Propose), arg0)
}

// QuorumSize mocks base method.
func (m *MockConfiguration) QuorumSize() int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "QuorumSize")
	ret0, _ := ret[0].(int)
	return ret0
}

// QuorumSize indicates an expected call of QuorumSize.
func (mr *MockConfigurationMockRecorder) QuorumSize() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "QuorumSize", reflect.TypeOf((*MockConfiguration)(nil).QuorumSize))
}

// Replica mocks base method.
func (m *MockConfiguration) Replica(arg0 hotstuff.ID) (modules.Replica, bool) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Replica", arg0)
	ret0, _ := ret[0].(modules.Replica)
	ret1, _ := ret[1].(bool)
	return ret0, ret1
}

// Replica indicates an expected call of Replica.
func (mr *MockConfigurationMockRecorder) Replica(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Replica", reflect.TypeOf((*MockConfiguration)(nil).Replica), arg0)
}

// Replicas mocks base method.
func (m *MockConfiguration) Replicas() map[hotstuff.ID]modules.Replica {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Replicas")
	ret0, _ := ret[0].(map[hotstuff.ID]modules.Replica)
	return ret0
}

// Replicas indicates an expected call of Replicas.
func (mr *MockConfigurationMockRecorder) Replicas() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Replicas", reflect.TypeOf((*MockConfiguration)(nil).Replicas))
}

// SubConfig mocks base method.
func (m *MockConfiguration) SubConfig(arg0 []hotstuff.ID) (modules.Configuration, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SubConfig", arg0)
	ret0, _ := ret[0].(modules.Configuration)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SubConfig indicates an expected call of SubConfig.
func (mr *MockConfigurationMockRecorder) SubConfig(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SubConfig", reflect.TypeOf((*MockConfiguration)(nil).SubConfig), arg0)
}

// Timeout mocks base method.
func (m *MockConfiguration) Timeout(arg0 msg.TimeoutMsg) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Timeout", arg0)
}

// Timeout indicates an expected call of Timeout.
func (mr *MockConfigurationMockRecorder) Timeout(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Timeout", reflect.TypeOf((*MockConfiguration)(nil).Timeout), arg0)
}
