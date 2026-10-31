//go:build darwin && !server

package wailsui

/*
#cgo CFLAGS: -mmacosx-version-min=10.13 -x objective-c
#cgo LDFLAGS: -framework Foundation -framework WebKit

#import <Foundation/Foundation.h>
#import <WebKit/WebKit.h>
#import <objc/message.h>
#import <objc/runtime.h>

typedef id (*webViewInitFn)(id, SEL, NSRect, WKWebViewConfiguration *);

static webViewInitFn originalWebViewInit;

static void disableNear60FPSPreference(WKPreferences *preferences) {
	SEL featuresSelector = NSSelectorFromString(@"_features");
	SEL setEnabledSelector = NSSelectorFromString(@"_setEnabled:forFeature:");
	if (![WKPreferences respondsToSelector:featuresSelector] || ![preferences respondsToSelector:setEnabledSelector]) {
		NSLog(@"[Hive] WebKit has no feature SPI; the webview stays near 60fps");
		return;
	}
	NSArray *features = ((NSArray *(*)(id, SEL))objc_msgSend)([WKPreferences class], featuresSelector);
	for (id feature in features) {
		if ([[feature valueForKey:@"key"] isEqual:@"PreferPageRenderingUpdatesNear60FPSEnabled"]) {
			((void (*)(id, SEL, BOOL, id))objc_msgSend)(preferences, setEnabledSelector, NO, feature);
			return;
		}
	}
	NSLog(@"[Hive] WebKit dropped its near-60fps preference; the webview's frame rate is WebKit's default");
}

static id webViewInitAtDisplayRefresh(id self, SEL cmd, NSRect frame, WKWebViewConfiguration *configuration) {
	// An exception here would escape window creation and end the app at launch.
	// The frame rate is not worth that when a macOS update changes this SPI.
	@try {
		disableNear60FPSPreference(configuration.preferences);
	} @catch (NSException *exception) {
		NSLog(@"[Hive] Could not change the webview frame rate: %@", exception.reason);
	}
	return originalWebViewInit(self, cmd, frame, configuration);
}

static void swizzleWebViewInit(void) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		Method method = class_getInstanceMethod([WKWebView class], @selector(initWithFrame:configuration:));
		originalWebViewInit = (webViewInitFn)method_setImplementation(method, (IMP)webViewInitAtDisplayRefresh);
	});
}
*/
import "C"

// unlockDisplayRefreshRate lets every webview render at the display's refresh
// rate rather than WebKit's default of about 60fps. Wails creates a webview and
// starts its first load in one main-thread call, and WebKit ignores this
// preference once a page has loaded, so it has to be set inside the initializer
// (ADR the-webview-renders-at-the-display-refresh-rate-through-a-webkit-private-preference).
func unlockDisplayRefreshRate() {
	C.swizzleWebViewInit()
}
