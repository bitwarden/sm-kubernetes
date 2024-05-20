// Code generated by MockGen. DO NOT EDIT.
// Source: bw-sdk/bitwarden_client.go
//
// Generated by this command:
//
//	mockgen -source bw-sdk/bitwarden_client.go -destination internal/controller/bitwardenclient_mock.go
//

// Package mock_sdk is a generated GoMock package.
package controller_test_mocks

import (
	reflect "reflect"

	sdk "github.com/tangowithfoxtrot/go-module-test"
	gomock "go.uber.org/mock/gomock"
)

// MockBitwardenClientInterface is a mock of BitwardenClientInterface interface.
type MockBitwardenClientInterface struct {
	ctrl     *gomock.Controller
	recorder *MockBitwardenClientInterfaceMockRecorder
}

// MockBitwardenClientInterfaceMockRecorder is the mock recorder for MockBitwardenClientInterface.
type MockBitwardenClientInterfaceMockRecorder struct {
	mock *MockBitwardenClientInterface
}

// NewMockBitwardenClientInterface creates a new mock instance.
func NewMockBitwardenClientInterface(ctrl *gomock.Controller) *MockBitwardenClientInterface {
	mock := &MockBitwardenClientInterface{ctrl: ctrl}
	mock.recorder = &MockBitwardenClientInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBitwardenClientInterface) EXPECT() *MockBitwardenClientInterfaceMockRecorder {
	return m.recorder
}

// AccessTokenLogin mocks base method.
func (m *MockBitwardenClientInterface) AccessTokenLogin(accessToken string, statePath *string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AccessTokenLogin", accessToken, statePath)
	ret0, _ := ret[0].(error)
	return ret0
}

// AccessTokenLogin indicates an expected call of AccessTokenLogin.
func (mr *MockBitwardenClientInterfaceMockRecorder) AccessTokenLogin(accessToken, statePath any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AccessTokenLogin", reflect.TypeOf((*MockBitwardenClientInterface)(nil).AccessTokenLogin), accessToken, statePath)
}

// Close mocks base method.
func (m *MockBitwardenClientInterface) Close() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Close")
}

// Close indicates an expected call of Close.
func (mr *MockBitwardenClientInterfaceMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockBitwardenClientInterface)(nil).Close))
}

// GetProjects mocks base method.
func (m *MockBitwardenClientInterface) GetProjects() sdk.ProjectsInterface {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetProjects")
	ret0, _ := ret[0].(sdk.ProjectsInterface)
	return ret0
}

// GetProjects indicates an expected call of GetProjects.
func (mr *MockBitwardenClientInterfaceMockRecorder) GetProjects() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetProjects", reflect.TypeOf((*MockBitwardenClientInterface)(nil).GetProjects))
}

// GetSecrets mocks base method.
func (m *MockBitwardenClientInterface) GetSecrets() sdk.SecretsInterface {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSecrets")
	ret0, _ := ret[0].(sdk.SecretsInterface)
	return ret0
}

// GetSecrets indicates an expected call of GetSecrets.
func (mr *MockBitwardenClientInterfaceMockRecorder) GetSecrets() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSecrets", reflect.TypeOf((*MockBitwardenClientInterface)(nil).GetSecrets))
}
