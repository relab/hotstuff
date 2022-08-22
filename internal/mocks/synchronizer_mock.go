// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/relab/hotstuff/modules (interfaces: Synchronizer)

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	hotstuff "github.com/relab/hotstuff"
)

// MockSynchronizer is a mock of Synchronizer interface.
type MockSynchronizer struct {
	ctrl     *gomock.Controller
	recorder *MockSynchronizerMockRecorder
}

// MockSynchronizerMockRecorder is the mock recorder for MockSynchronizer.
type MockSynchronizerMockRecorder struct {
	mock *MockSynchronizer
}

// NewMockSynchronizer creates a new mock instance.
func NewMockSynchronizer(ctrl *gomock.Controller) *MockSynchronizer {
	mock := &MockSynchronizer{ctrl: ctrl}
	mock.recorder = &MockSynchronizerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSynchronizer) EXPECT() *MockSynchronizerMockRecorder {
	return m.recorder
}

// AdvanceView mocks base method.
func (m *MockSynchronizer) AdvanceView(arg0 hotstuff.SyncInfo) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AdvanceView", arg0)
}

// AdvanceView indicates an expected call of AdvanceView.
func (mr *MockSynchronizerMockRecorder) AdvanceView(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AdvanceView", reflect.TypeOf((*MockSynchronizer)(nil).AdvanceView), arg0)
}

// HighQC mocks base method.
func (m *MockSynchronizer) HighQC() hotstuff.QuorumCert {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HighQC")
	ret0, _ := ret[0].(hotstuff.QuorumCert)
	return ret0
}

// HighQC indicates an expected call of HighQC.
func (mr *MockSynchronizerMockRecorder) HighQC() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HighQC", reflect.TypeOf((*MockSynchronizer)(nil).HighQC))
}

// LeafBlock mocks base method.
func (m *MockSynchronizer) LeafBlock() *hotstuff.Block {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "LeafBlock")
	ret0, _ := ret[0].(*hotstuff.Block)
	return ret0
}

// LeafBlock indicates an expected call of LeafBlock.
func (mr *MockSynchronizerMockRecorder) LeafBlock() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LeafBlock", reflect.TypeOf((*MockSynchronizer)(nil).LeafBlock))
}

// Start mocks base method.
func (m *MockSynchronizer) Start(arg0 context.Context) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Start", arg0)
}

// Start indicates an expected call of Start.
func (mr *MockSynchronizerMockRecorder) Start(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Start", reflect.TypeOf((*MockSynchronizer)(nil).Start), arg0)
}

// View mocks base method.
func (m *MockSynchronizer) View() hotstuff.View {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "View")
	ret0, _ := ret[0].(hotstuff.View)
	return ret0
}

// View indicates an expected call of View.
func (mr *MockSynchronizerMockRecorder) View() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "View", reflect.TypeOf((*MockSynchronizer)(nil).View))
}

// ViewContext mocks base method.
func (m *MockSynchronizer) ViewContext() context.Context {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ViewContext")
	ret0, _ := ret[0].(context.Context)
	return ret0
}

// ViewContext indicates an expected call of ViewContext.
func (mr *MockSynchronizerMockRecorder) ViewContext() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ViewContext", reflect.TypeOf((*MockSynchronizer)(nil).ViewContext))
}
