// Package metrics contains modules that collect data or metrics from other modules.
//
// The preferred way to collect metrics is to send an event from the target module onto the metrics event loop.
// The metrics event loop is a secondary event loop that is reserved for this purpose. It is available both for
// client modules and replica modules.
//
// A ticker is used to determine how often metrics should be logged. To receive tick events, you can add an observer
// for a types.TickEvent on the metrics logger. When receiving the tick event, you should write the relevant data
// to the MetricsLogger module as a protobuf message.
package metrics
