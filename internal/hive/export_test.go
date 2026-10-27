package hive

// WaitBackground blocks until the bus and the KV sweep have returned.
func (r *Runtime) WaitBackground() { r.wg.Wait() }
