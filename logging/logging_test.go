package logging

import (
	"testing"
)

func BenchmarkInnerLogger(b *testing.B) {
	SetLogLevel("error")
	logger := New("test").(*wrapper).inner

	for b.Loop() {
		logger.Info("test")
	}
}

func BenchmarkWrappedLoggerNoPackages(b *testing.B) {
	SetLogLevel("error")
	logger := New("test")

	for b.Loop() {
		logger.Info("test")
	}
}

func BenchmarkWrappedLoggerWithPackage(b *testing.B) {
	SetLogLevel("error")
	SetPackageLogLevel("foo", "error")
	logger := New("test")

	for b.Loop() {
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

	for b.Loop() {
		logger.Info("test")
	}
}
