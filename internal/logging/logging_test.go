package logging

import (
	"testing"
)

func BenchmarkInnerLogger(b *testing.B) {
	SetLogLevel("error")
	logger := New("test").(*wrapper).inner

	for i := 0; i < b.N; i++ {
		logger.Info("test")
	}
}

func BenchmarkWrappedLoggerNoPackages(b *testing.B) {
	SetLogLevel("error")
	logger := New("test")

	for i := 0; i < b.N; i++ {
		logger.Info("test")
	}
}

func BenchmarkWrappedLoggerWithPackage(b *testing.B) {
	SetLogLevel("error")
	SetPackageLogLevel("foo", "error")
	logger := New("test")

	for i := 0; i < b.N; i++ {
		logger.Info("test")
	}
}

func BenchmarkWrappedLoggerWithMultiplePackages(b *testing.B) {
	SetLogLevel("error")
	SetPackageLogLevel("foo", "error")
	SetPackageLogLevel("bar", "error")
	SetPackageLogLevel("baz", "error")
	SetPackageLogLevel("qux", "error")
	SetPackageLogLevel("quux", "error")
	logger := New("test")

	for i := 0; i < b.N; i++ {
		logger.Info("test")
	}
}
