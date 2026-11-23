package core

func SetBeforePut(d *Documents, f func()) { d.beforePut = f }
