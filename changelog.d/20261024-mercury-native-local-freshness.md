Restore independent local aging for the native Mercury dashboard. The view
now advances producer node and link observation ages from read completion with
a monotonic clock and withholds stale, future, failed-read, and unknown values
without another sampler or request path. Mercury dashboard v1 does not expose
power telemetry; it remains explicitly unknown, with the broader native
telemetry obligation open.
