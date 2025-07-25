// Code generated by mockery; DO NOT EDIT.
// github.com/vektra/mockery
// template: testify

package messaging

import (
	"context"

	mock "github.com/stretchr/testify/mock"
)

// NewMockPublisher creates a new instance of MockPublisher. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockPublisher(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockPublisher {
	mock := &MockPublisher{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}

// MockPublisher is an autogenerated mock type for the Publisher type
type MockPublisher struct {
	mock.Mock
}

type MockPublisher_Expecter struct {
	mock *mock.Mock
}

func (_m *MockPublisher) EXPECT() *MockPublisher_Expecter {
	return &MockPublisher_Expecter{mock: &_m.Mock}
}

// Publish provides a mock function for the type MockPublisher
func (_mock *MockPublisher) Publish(ctx context.Context, subject string, messageID string, message []byte) error {
	ret := _mock.Called(ctx, subject, messageID, message)

	if len(ret) == 0 {
		panic("no return value specified for Publish")
	}

	var r0 error
	if returnFunc, ok := ret.Get(0).(func(context.Context, string, string, []byte) error); ok {
		r0 = returnFunc(ctx, subject, messageID, message)
	} else {
		r0 = ret.Error(0)
	}
	return r0
}

// MockPublisher_Publish_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Publish'
type MockPublisher_Publish_Call struct {
	*mock.Call
}

// Publish is a helper method to define mock.On call
//   - ctx context.Context
//   - subject string
//   - messageID string
//   - message []byte
func (_e *MockPublisher_Expecter) Publish(ctx interface{}, subject interface{}, messageID interface{}, message interface{}) *MockPublisher_Publish_Call {
	return &MockPublisher_Publish_Call{Call: _e.mock.On("Publish", ctx, subject, messageID, message)}
}

func (_c *MockPublisher_Publish_Call) Run(run func(ctx context.Context, subject string, messageID string, message []byte)) *MockPublisher_Publish_Call {
	_c.Call.Run(func(args mock.Arguments) {
		var arg0 context.Context
		if args[0] != nil {
			arg0 = args[0].(context.Context)
		}
		var arg1 string
		if args[1] != nil {
			arg1 = args[1].(string)
		}
		var arg2 string
		if args[2] != nil {
			arg2 = args[2].(string)
		}
		var arg3 []byte
		if args[3] != nil {
			arg3 = args[3].([]byte)
		}
		run(
			arg0,
			arg1,
			arg2,
			arg3,
		)
	})
	return _c
}

func (_c *MockPublisher_Publish_Call) Return(err error) *MockPublisher_Publish_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *MockPublisher_Publish_Call) RunAndReturn(run func(ctx context.Context, subject string, messageID string, message []byte) error) *MockPublisher_Publish_Call {
	_c.Call.Return(run)
	return _c
}
