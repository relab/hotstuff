// Package metrics contains modules that collect data or metrics from other modules.
//
// The preferred way to collect metrics is to send an event from the target module onto the event loop,
// which can then be received by a metric module for processing before being written to a data file using the
// metrics logger.
//
// A ticker is used to determine how often metrics should be logged. To receive tick events, you can add an observer
// for a types.TickEvent on the metrics logger. When receiving the tick event, you should write the relevant data
// to the MetricsLogger module as a protobuf message.
//
// The event loop is accessed through the EventLoop() method of the module system,
// and the metrics logger is accessed through the MetricsLogger() method of the module system.
package metrics
