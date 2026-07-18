//go:build android

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// Android c-shared builds do not invoke main automatically.
var _ = func() struct{} {
	application.RegisterAndroidMain(main)
	return struct{}{}
}()
