//go:build darwin && ghostty && !server

// A libghostty surface in an NSView layered over the Wails webview. Input
// handling follows Ghostty's own AppKit host (macos/Sources/Ghostty/Surface
// View/SurfaceView_AppKit.swift), reduced to what one embedded pane needs.

#import <Cocoa/Cocoa.h>
#import <Carbon/Carbon.h>
#import <QuartzCore/QuartzCore.h>
#import <WebKit/WebKit.h>
#import <IOSurface/IOSurface.h>
#include "ghostty.h"
#include "host_darwin.h"

extern void hiveNativeTermEvent(uintptr_t handle, int kind, char *data);

static ghostty_app_t gApp;
static ghostty_config_t gConfig;
// Page elements floating over the terminals (menus, popovers, dialogs), in
// webview coordinates. A surface masks them out so the page shows through.
static NSArray<NSValue *> *gOccluders;
static NSMutableDictionary<NSNumber *, NSView *> *gViews;

static void emit(uintptr_t handle, int kind, NSString *data) {
	hiveNativeTermEvent(handle, kind, (char *)(data ? data.UTF8String : ""));
}

static ghostty_input_mods_e modsOf(NSEventModifierFlags f) {
	uint32_t m = GHOSTTY_MODS_NONE;
	if (f & NSEventModifierFlagShift) m |= GHOSTTY_MODS_SHIFT;
	if (f & NSEventModifierFlagControl) m |= GHOSTTY_MODS_CTRL;
	if (f & NSEventModifierFlagOption) m |= GHOSTTY_MODS_ALT;
	if (f & NSEventModifierFlagCommand) m |= GHOSTTY_MODS_SUPER;
	if (f & NSEventModifierFlagCapsLock) m |= GHOSTTY_MODS_CAPS;
	if (f & NX_DEVICERSHIFTKEYMASK) m |= GHOSTTY_MODS_SHIFT_RIGHT;
	if (f & NX_DEVICERCTLKEYMASK) m |= GHOSTTY_MODS_CTRL_RIGHT;
	if (f & NX_DEVICERALTKEYMASK) m |= GHOSTTY_MODS_ALT_RIGHT;
	if (f & NX_DEVICERCMDKEYMASK) m |= GHOSTTY_MODS_SUPER_RIGHT;
	return (ghostty_input_mods_e)m;
}

static NSEventModifierFlags flagsOf(ghostty_input_mods_e m) {
	NSEventModifierFlags f = 0;
	if (m & GHOSTTY_MODS_SHIFT) f |= NSEventModifierFlagShift;
	if (m & GHOSTTY_MODS_CTRL) f |= NSEventModifierFlagControl;
	if (m & GHOSTTY_MODS_ALT) f |= NSEventModifierFlagOption;
	if (m & GHOSTTY_MODS_SUPER) f |= NSEventModifierFlagCommand;
	return f;
}

// The text Ghostty should see for a key: control characters are re-derived
// without Ctrl (Ghostty encodes those itself), and the private-use function
// key range carries no text.
static NSString *keyText(NSEvent *e) {
	NSString *chars = e.characters;
	if (chars.length == 1) {
		unichar c = [chars characterAtIndex:0];
		if (c < 0x20) return [e charactersByApplyingModifiers:(e.modifierFlags & ~NSEventModifierFlagControl)];
		if (c >= 0xF700 && c <= 0xF8FF) return nil;
	}
	return chars;
}

static ghostty_input_key_s keyEvent(NSEvent *e, ghostty_input_action_e action, NSEventModifierFlags translation) {
	ghostty_input_key_s k = {0};
	k.action = action;
	k.keycode = e.keyCode;
	k.mods = modsOf(e.modifierFlags);
	k.consumed_mods = modsOf(translation & ~(NSEventModifierFlagControl | NSEventModifierFlagCommand));
	if (e.type == NSEventTypeKeyDown || e.type == NSEventTypeKeyUp) {
		NSString *u = [e charactersByApplyingModifiers:0];
		if (u.length > 0) k.unshifted_codepoint = [u characterAtIndex:0];
	}
	return k;
}

static NSView *webviewOf(NSWindow *window);

@interface HiveGhosttyView : NSView <NSTextInputClient>
@property(nonatomic) ghostty_surface_t surface;
@property(nonatomic) uintptr_t handle;
@property(nonatomic, strong) NSMutableArray<NSString *> *keyTextAccumulator;
@property(nonatomic, strong) NSMutableAttributedString *marked;
@property(nonatomic, strong) NSTrackingArea *tracking;
@property(nonatomic) BOOL focused;
@property(nonatomic) NSSize reportedCell;
@end

@implementation HiveGhosttyView

- (instancetype)initWithFrame:(NSRect)frame handle:(uintptr_t)handle {
	self = [super initWithFrame:frame];
	if (self) {
		_handle = handle;
		_marked = [[NSMutableAttributedString alloc] init];
	}
	return self;
}

- (BOOL)acceptsFirstResponder { return YES; }
- (BOOL)isFlipped { return NO; }
// Wails makes the window movable by its background, and a layer-backed view
// that is not opaque counts as background: a click would drag the window
// instead of reaching the terminal.
- (BOOL)mouseDownCanMoveWindow { return NO; }
- (BOOL)acceptsFirstMouse:(NSEvent *)event { return YES; }

- (BOOL)becomeFirstResponder {
	BOOL ok = [super becomeFirstResponder];
	if (ok) [self setFocus:YES];
	return ok;
}

- (BOOL)resignFirstResponder {
	BOOL ok = [super resignFirstResponder];
	if (ok) [self setFocus:NO];
	return ok;
}

- (void)setFocus:(BOOL)focused {
	if (_focused == focused) return;
	_focused = focused;
	if (_surface) ghostty_surface_set_focus(_surface, focused);
	emit(_handle, HIVE_NT_FOCUS, focused ? @"1" : @"0");
}

- (void)setFrameSize:(NSSize)size {
	[super setFrameSize:size];
	[self syncSize];
	[self applyMask];
}

- (void)syncSize {
	if (!_surface) return;
	NSSize px = [self convertSizeToBacking:self.frame.size];
	if (px.width >= 1 && px.height >= 1) ghostty_surface_set_size(_surface, (uint32_t)px.width, (uint32_t)px.height);
	[self reportCell];
}

// The cell in points: what the page sizes a tmux pane's box by, so the grid
// tmux sets is the grid this surface draws.
- (void)reportCell {
	ghostty_surface_size_s size = ghostty_surface_size(_surface);
	CGFloat scale = self.window.backingScaleFactor ?: 1;
	NSSize cell = NSMakeSize(size.cell_width_px / scale, size.cell_height_px / scale);
	if (cell.width <= 0 || NSEqualSizes(cell, _reportedCell)) return;
	_reportedCell = cell;
	emit(_handle, HIVE_NT_CELL, [NSString stringWithFormat:@"%g,%g", cell.width, cell.height]);
}

// Ghostty paces its renderer with a CVDisplayLink on this view's screen. It
// creates the link only once it is told the display: without one it draws a
// frame for every chunk of output, which is what made full-screen programs
// stutter.
- (void)syncDisplay {
	NSScreen *screen = self.window.screen;
	if (!_surface || !screen) return;
	NSNumber *display = screen.deviceDescription[@"NSScreenNumber"];
	ghostty_surface_set_display_id(_surface, display.unsignedIntValue);
}

- (void)viewDidMoveToWindow {
	[super viewDidMoveToWindow];
	[NSNotificationCenter.defaultCenter removeObserver:self name:NSWindowDidChangeScreenNotification object:nil];
	if (!self.window) return;
	[NSNotificationCenter.defaultCenter addObserver:self
	                                       selector:@selector(windowDidChangeScreen:)
	                                           name:NSWindowDidChangeScreenNotification
	                                         object:self.window];
	[self syncDisplay];
}

- (void)windowDidChangeScreen:(NSNotification *)note {
	[self syncDisplay];
	[self viewDidChangeBackingProperties];
}

- (void)viewDidChangeBackingProperties {
	[super viewDidChangeBackingProperties];
	if (self.window) {
		[CATransaction begin];
		[CATransaction setDisableActions:YES];
		self.layer.contentsScale = self.window.backingScaleFactor;
		[CATransaction commit];
	}
	if (!_surface) return;
	NSRect fb = [self convertRectToBacking:self.frame];
	if (self.frame.size.width > 0 && self.frame.size.height > 0) {
		ghostty_surface_set_content_scale(_surface, fb.size.width / self.frame.size.width,
		                                  fb.size.height / self.frame.size.height);
	}
	[self syncSize];
}

- (void)updateTrackingAreas {
	if (_tracking) [self removeTrackingArea:_tracking];
	_tracking = [[NSTrackingArea alloc]
	    initWithRect:self.bounds
	         options:NSTrackingMouseEnteredAndExited | NSTrackingMouseMoved | NSTrackingInVisibleRect |
	                 NSTrackingActiveAlways
	           owner:self
	        userInfo:nil];
	[self addTrackingArea:_tracking];
	[super updateTrackingAreas];
}

#pragma mark Occlusion

// The occluders that overlap this view, in its own coordinates.
- (NSArray<NSValue *> *)localOccluders {
	NSView *web = webviewOf(self.window);
	NSMutableArray<NSValue *> *local = [NSMutableArray array];
	for (NSValue *v in gOccluders) {
		NSRect r = NSIntersectionRect([web convertRect:v.rectValue toView:self], self.bounds);
		if (!NSIsEmptyRect(r)) [local addObject:[NSValue valueWithRect:r]];
	}
	return local;
}

// Cuts the occluders out of the drawn layer. A bitmap mask rather than a
// path, so overlapping rectangles stay cut.
- (void)applyMask {
	NSArray<NSValue *> *rects = self.localOccluders;
	CALayer *layer = self.layer;
	if (!layer) return;
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	if (rects.count == 0) {
		layer.mask = nil;
	} else {
		size_t w = MAX((size_t)ceil(self.bounds.size.width), 1), h = MAX((size_t)ceil(self.bounds.size.height), 1);
		CGColorSpaceRef gray = CGColorSpaceCreateDeviceGray();
		CGContextRef ctx = CGBitmapContextCreate(NULL, w, h, 8, 0, gray, (CGBitmapInfo)kCGImageAlphaOnly);
		CGColorSpaceRelease(gray);
		CGContextSetAlpha(ctx, 1);
		CGContextFillRect(ctx, CGRectMake(0, 0, w, h));
		for (NSValue *v in rects) CGContextClearRect(ctx, NSRectToCGRect(v.rectValue));
		CGImageRef image = CGBitmapContextCreateImage(ctx);
		CGContextRelease(ctx);
		CALayer *mask = [CALayer layer];
		mask.frame = layer.bounds;
		mask.contents = (__bridge id)image;
		mask.contentsGravity = kCAGravityResize;
		CGImageRelease(image);
		layer.mask = mask;
	}
	[CATransaction commit];
}

// A click on a cut-out goes to the page beneath.
- (NSView *)hitTest:(NSPoint)point {
	NSView *hit = [super hitTest:point];
	if (hit != self) return hit;
	NSPoint local = [self convertPoint:point fromView:self.superview];
	for (NSValue *v in self.localOccluders) {
		if (NSPointInRect(local, v.rectValue)) return nil;
	}
	return hit;
}

#pragma mark Mouse

- (void)sendMousePos:(NSEvent *)e {
	if (!_surface) return;
	NSPoint p = [self convertPoint:e.locationInWindow fromView:nil];
	ghostty_surface_mouse_pos(_surface, p.x, self.frame.size.height - p.y, modsOf(e.modifierFlags));
}

- (void)mouseDown:(NSEvent *)e {
	if (!_focused) [self.window makeFirstResponder:self];
	[self sendMousePos:e];
	if (_surface) ghostty_surface_mouse_button(_surface, GHOSTTY_MOUSE_PRESS, GHOSTTY_MOUSE_LEFT, modsOf(e.modifierFlags));
}
- (void)mouseUp:(NSEvent *)e {
	if (!_surface) return;
	ghostty_surface_mouse_button(_surface, GHOSTTY_MOUSE_RELEASE, GHOSTTY_MOUSE_LEFT, modsOf(e.modifierFlags));
	ghostty_surface_mouse_pressure(_surface, 0, 0);
}
- (void)rightMouseDown:(NSEvent *)e {
	if (!_surface || !ghostty_surface_mouse_button(_surface, GHOSTTY_MOUSE_PRESS, GHOSTTY_MOUSE_RIGHT, modsOf(e.modifierFlags)))
		[super rightMouseDown:e];
}
- (void)rightMouseUp:(NSEvent *)e {
	if (!_surface || !ghostty_surface_mouse_button(_surface, GHOSTTY_MOUSE_RELEASE, GHOSTTY_MOUSE_RIGHT, modsOf(e.modifierFlags)))
		[super rightMouseUp:e];
}
- (void)otherMouseDown:(NSEvent *)e {
	if (_surface && e.buttonNumber == 2) ghostty_surface_mouse_button(_surface, GHOSTTY_MOUSE_PRESS, GHOSTTY_MOUSE_MIDDLE, modsOf(e.modifierFlags));
}
- (void)otherMouseUp:(NSEvent *)e {
	if (_surface && e.buttonNumber == 2) ghostty_surface_mouse_button(_surface, GHOSTTY_MOUSE_RELEASE, GHOSTTY_MOUSE_MIDDLE, modsOf(e.modifierFlags));
}
- (void)mouseMoved:(NSEvent *)e { [self sendMousePos:e]; }
- (void)mouseDragged:(NSEvent *)e { [self sendMousePos:e]; }
- (void)rightMouseDragged:(NSEvent *)e { [self sendMousePos:e]; }
- (void)otherMouseDragged:(NSEvent *)e { [self sendMousePos:e]; }
- (void)mouseExited:(NSEvent *)e {
	// Ghostty reads a negative position as "outside": link hover ends.
	if (_surface && NSEvent.pressedMouseButtons == 0) ghostty_surface_mouse_pos(_surface, -1, -1, modsOf(e.modifierFlags));
}

- (void)scrollWheel:(NSEvent *)e {
	if (!_surface) return;
	double x = e.scrollingDeltaX, y = e.scrollingDeltaY;
	int mods = 0;
	if (e.hasPreciseScrollingDeltas) {
		x *= 2;
		y *= 2;
		mods |= 1;
	}
	int momentum = 0;
	switch (e.momentumPhase) {
	case NSEventPhaseBegan: momentum = 1; break;
	case NSEventPhaseStationary: momentum = 2; break;
	case NSEventPhaseChanged: momentum = 3; break;
	case NSEventPhaseEnded: momentum = 4; break;
	case NSEventPhaseCancelled: momentum = 5; break;
	case NSEventPhaseMayBegin: momentum = 6; break;
	default: break;
	}
	mods |= momentum << 1;
	ghostty_surface_mouse_scroll(_surface, x, y, mods);
}

- (void)pressureChangeWithEvent:(NSEvent *)e {
	if (_surface) ghostty_surface_mouse_pressure(_surface, (uint32_t)e.stage, e.pressure);
}

#pragma mark Keyboard

// A ⌘ chord that is not one of the surface's own bindings (copy, paste)
// belongs to the app's keymap in the webview, which cannot see keys while
// this view is first responder.
- (BOOL)performKeyEquivalent:(NSEvent *)e {
	if (e.type != NSEventTypeKeyDown || self.window.firstResponder != self || !_surface) return NO;
	if (!(e.modifierFlags & NSEventModifierFlagCommand)) return NO;
	ghostty_input_key_s k = keyEvent(e, GHOSTTY_ACTION_PRESS, e.modifierFlags);
	NSString *text = keyText(e);
	if (text.length > 0) k.text = text.UTF8String;
	if (ghostty_surface_key_is_binding(_surface, k, NULL)) {
		[self keyDown:e];
		return YES;
	}
	[self forwardChord:e];
	return YES;
}

- (void)forwardChord:(NSEvent *)e {
	NSString *key = [e charactersByApplyingModifiers:0] ?: @"";
	NSDictionary *chord = @{
		@"key" : key,
		@"keyCode" : @(e.keyCode),
		@"meta" : @((e.modifierFlags & NSEventModifierFlagCommand) != 0),
		@"ctrl" : @((e.modifierFlags & NSEventModifierFlagControl) != 0),
		@"alt" : @((e.modifierFlags & NSEventModifierFlagOption) != 0),
		@"shift" : @((e.modifierFlags & NSEventModifierFlagShift) != 0),
	};
	NSData *json = [NSJSONSerialization dataWithJSONObject:chord options:0 error:nil];
	emit(_handle, HIVE_NT_KEY, [[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding]);
}

- (void)keyDown:(NSEvent *)e {
	if (!_surface) {
		[self interpretKeyEvents:@[ e ]];
		return;
	}
	NSEventModifierFlags translation =
	    flagsOf(ghostty_surface_key_translation_mods(_surface, modsOf(e.modifierFlags)));
	NSEventModifierFlags mods = e.modifierFlags;
	for (NSNumber *n in @[ @(NSEventModifierFlagShift), @(NSEventModifierFlagControl), @(NSEventModifierFlagOption),
	                        @(NSEventModifierFlagCommand) ]) {
		NSEventModifierFlags f = n.unsignedIntegerValue;
		if (translation & f) mods |= f;
		else mods &= ~f;
	}
	NSEvent *te = e;
	if (mods != e.modifierFlags) {
		te = [NSEvent keyEventWithType:e.type
		                      location:e.locationInWindow
		                 modifierFlags:mods
		                     timestamp:e.timestamp
		                  windowNumber:e.windowNumber
		                       context:nil
		                    characters:[e charactersByApplyingModifiers:mods] ?: @""
		   charactersIgnoringModifiers:e.charactersIgnoringModifiers ?: @""
		                     isARepeat:e.isARepeat
		                       keyCode:e.keyCode] ?: e;
	}
	ghostty_input_action_e action = e.isARepeat ? GHOSTTY_ACTION_REPEAT : GHOSTTY_ACTION_PRESS;
	BOOL markedBefore = _marked.length > 0;

	_keyTextAccumulator = [NSMutableArray array];
	[self interpretKeyEvents:@[ te ]];
	NSArray<NSString *> *texts = _keyTextAccumulator;
	_keyTextAccumulator = nil;

	[self syncPreedit:markedBefore];
	BOOL composing = _marked.length > 0 || markedBefore;

	if (texts.count > 0) {
		for (NSString *t in texts) {
			if (composing && t.length == 1 && [t characterAtIndex:0] < 0x20) continue;
			[self sendKey:e action:action mods:te.modifierFlags text:t composing:NO];
		}
		return;
	}
	NSString *t = keyText(te);
	if (composing && t.length == 1 && [t characterAtIndex:0] < 0x20) return;
	[self sendKey:e action:action mods:te.modifierFlags text:t composing:composing];
}

- (void)keyUp:(NSEvent *)e {
	[self sendKey:e action:GHOSTTY_ACTION_RELEASE mods:e.modifierFlags text:nil composing:NO];
}

- (void)flagsChanged:(NSEvent *)e {
	uint32_t mod;
	switch (e.keyCode) {
	case 0x39: mod = GHOSTTY_MODS_CAPS; break;
	case 0x38: case 0x3C: mod = GHOSTTY_MODS_SHIFT; break;
	case 0x3B: case 0x3E: mod = GHOSTTY_MODS_CTRL; break;
	case 0x3A: case 0x3D: mod = GHOSTTY_MODS_ALT; break;
	case 0x37: case 0x36: mod = GHOSTTY_MODS_SUPER; break;
	default: return;
	}
	if (_marked.length > 0) return;
	ghostty_input_action_e action = (modsOf(e.modifierFlags) & mod) ? GHOSTTY_ACTION_PRESS : GHOSTTY_ACTION_RELEASE;
	[self sendKey:e action:action mods:e.modifierFlags text:nil composing:NO];
}

- (void)sendKey:(NSEvent *)e
         action:(ghostty_input_action_e)action
           mods:(NSEventModifierFlags)translation
           text:(NSString *)text
      composing:(BOOL)composing {
	if (!_surface) return;
	ghostty_input_key_s k = keyEvent(e, action, translation);
	k.composing = composing;
	if (text.length > 0) {
		k.text = text.UTF8String;
	}
	bool handled = ghostty_surface_key(_surface, k);
}

- (void)syncPreedit:(BOOL)clearIfNeeded {
	if (!_surface) return;
	if (_marked.length > 0) {
		const char *s = _marked.string.UTF8String;
		ghostty_surface_preedit(_surface, s, strlen(s));
	} else if (clearIfNeeded) {
		ghostty_surface_preedit(_surface, NULL, 0);
	}
}

#pragma mark NSTextInputClient

- (void)insertText:(id)string replacementRange:(NSRange)range {
	// Copied: the input context reuses a mutable string and clears it after
	// this call, before keyDown sends the text on.
	NSString *s = [([string isKindOfClass:NSAttributedString.class] ? [string string] : string) copy];
	[self unmarkText];
	if (_keyTextAccumulator) {
		[_keyTextAccumulator addObject:s];
		return;
	}
	// Text from outside a key event (dictation, the character palette).
	if (_surface && s.length > 0) ghostty_surface_text(_surface, s.UTF8String, strlen(s.UTF8String));
}

- (void)setMarkedText:(id)string selectedRange:(NSRange)sel replacementRange:(NSRange)rep {
	NSString *s = [string isKindOfClass:NSAttributedString.class] ? [string string] : string;
	[_marked.mutableString setString:s];
	if (!_keyTextAccumulator) [self syncPreedit:YES];
}

- (void)unmarkText {
	if (_marked.length == 0) return;
	[_marked.mutableString setString:@""];
	[self syncPreedit:YES];
}

- (BOOL)hasMarkedText { return _marked.length > 0; }
- (NSRange)markedRange { return _marked.length > 0 ? NSMakeRange(0, _marked.length) : NSMakeRange(NSNotFound, 0); }
- (NSRange)selectedRange { return NSMakeRange(NSNotFound, 0); }
- (NSArray<NSAttributedStringKey> *)validAttributesForMarkedText { return @[]; }
- (NSAttributedString *)attributedSubstringForProposedRange:(NSRange)r actualRange:(NSRangePointer)a { return nil; }
- (NSUInteger)characterIndexForPoint:(NSPoint)p { return 0; }

- (NSRect)firstRectForCharacterRange:(NSRange)r actualRange:(NSRangePointer)a {
	double x = 0, y = 0, w = 0, h = 0;
	if (_surface) ghostty_surface_ime_point(_surface, &x, &y, &w, &h);
	NSRect local = NSMakeRect(x, self.frame.size.height - y, w, MAX(h, 1));
	NSRect win = [self convertRect:local toView:nil];
	return [self.window convertRectToScreen:win];
}

// Keys interpretKeyEvents maps to commands (Enter, Backspace, arrows) are
// sent as key events by keyDown; nothing to do here.
- (void)doCommandBySelector:(SEL)selector {
}

@end

#pragma mark Runtime callbacks

static HiveGhosttyView *viewForUserdata(void *ud) { return (__bridge HiveGhosttyView *)ud; }

static void wakeup(void *ud) {
	dispatch_async(dispatch_get_main_queue(), ^{
	  if (gApp) ghostty_app_tick(gApp);
	});
}

static bool action(ghostty_app_t app, ghostty_target_s target, ghostty_action_s act) {
	HiveGhosttyView *view = nil;
	if (target.tag == GHOSTTY_TARGET_SURFACE && target.target.surface)
		view = viewForUserdata(ghostty_surface_userdata(target.target.surface));
	switch (act.tag) {
	case GHOSTTY_ACTION_SET_TITLE:
		if (view && act.action.set_title.title) emit(view.handle, HIVE_NT_TITLE, @(act.action.set_title.title));
		return true;
	case GHOSTTY_ACTION_OPEN_URL: {
		NSString *url = [[NSString alloc] initWithBytes:act.action.open_url.url
		                                          length:act.action.open_url.len
		                                        encoding:NSUTF8StringEncoding];
		if (view && url) emit(view.handle, HIVE_NT_OPEN_URL, url);
		return true;
	}
	case GHOSTTY_ACTION_MOUSE_SHAPE:
		switch (act.action.mouse_shape) {
		case GHOSTTY_MOUSE_SHAPE_POINTER: [[NSCursor pointingHandCursor] set]; break;
		case GHOSTTY_MOUSE_SHAPE_TEXT: [[NSCursor IBeamCursor] set]; break;
		default: [[NSCursor arrowCursor] set]; break;
		}
		return true;
	case GHOSTTY_ACTION_RENDER:
		return false;
	default:
		return false;
	}
}

static NSPasteboard *pasteboardFor(ghostty_clipboard_e loc) {
	return loc == GHOSTTY_CLIPBOARD_STANDARD ? NSPasteboard.generalPasteboard : nil;
}

static ghostty_clipboard_read_result_e readClipboard(void *ud, ghostty_clipboard_e loc, void *state,
                                                     const char *const *mimes, size_t mimesLen, bool list) {
	HiveGhosttyView *view = viewForUserdata(ud);
	NSPasteboard *pb = pasteboardFor(loc);
	if (!view.surface || !pb) return GHOSTTY_CLIPBOARD_READ_UNSUPPORTED;
	NSString *s = [pb stringForType:NSPasteboardTypeString];
	if (!s) return GHOSTTY_CLIPBOARD_READ_UNAVAILABLE;
	const char *data = s.UTF8String;
	ghostty_clipboard_content_s content = {.mime = "text/plain", .data = data, .len = strlen(data)};
	ghostty_clipboard_complete_s done = {.contents = &content, .contents_len = 1, .confirmed = false};
	ghostty_surface_complete_clipboard_request(view.surface, &done, state);
	return GHOSTTY_CLIPBOARD_READ_STARTED;
}

static void confirmReadClipboard(void *ud, const ghostty_clipboard_confirm_s *req, void *state,
                                 ghostty_clipboard_request_e kind) {
	// No prompt UI here: an unsafe paste or an OSC 52 read is refused.
	HiveGhosttyView *view = viewForUserdata(ud);
	if (view.surface) ghostty_surface_deny_clipboard_request(view.surface, state);
}

static void writeClipboard(void *ud, ghostty_clipboard_e loc, const ghostty_clipboard_content_s *content, size_t len,
                           bool confirm) {
	NSPasteboard *pb = pasteboardFor(loc);
	if (!pb) return;
	for (size_t i = 0; i < len; i++) {
		if (content[i].mime && strcmp(content[i].mime, "text/plain") != 0) continue;
		NSString *s = [[NSString alloc] initWithBytes:content[i].data length:content[i].len encoding:NSUTF8StringEncoding];
		if (!s) continue;
		[pb clearContents];
		[pb setString:s forType:NSPasteboardTypeString];
		return;
	}
}

static void closeSurface(void *ud, bool processAlive) {
	HiveGhosttyView *view = viewForUserdata(ud);
	uintptr_t handle = view.handle;
	dispatch_async(dispatch_get_main_queue(), ^{
	  emit(handle, HIVE_NT_CLOSED, nil);
	});
}

#pragma mark API

static bool initOnMain(const char *configPath, bool userConfig) {
	if (gApp) return true;
	char *argv[] = {"hive", NULL};
	if (ghostty_init(1, argv) != 0) return false;
	ghostty_config_t cfg = ghostty_config_new();
	if (userConfig) ghostty_config_load_default_files(cfg);
	if (configPath) ghostty_config_load_file(cfg, configPath);
	ghostty_config_finalize(cfg);
	ghostty_runtime_config_s rt = {
	    .userdata = NULL,
	    .supports_selection_clipboard = false,
	    .wakeup_cb = wakeup,
	    .action_cb = action,
	    .read_clipboard_cb = readClipboard,
	    .confirm_read_clipboard_cb = confirmReadClipboard,
	    .write_clipboard_cb = writeClipboard,
	    .close_surface_cb = closeSurface,
	};
	gApp = ghostty_app_new(&rt, cfg);
	gConfig = cfg;
	if (!gApp) return false;
	gViews = [NSMutableDictionary dictionary];
	ghostty_app_set_focus(gApp, NSApp.isActive);
	[NSNotificationCenter.defaultCenter addObserverForName:NSApplicationDidBecomeActiveNotification
	                                                object:nil
	                                                 queue:nil
	                                            usingBlock:^(NSNotification *n) {
	                                              ghostty_app_set_focus(gApp, true);
	                                            }];
	[NSNotificationCenter.defaultCenter addObserverForName:NSApplicationDidResignActiveNotification
	                                                object:nil
	                                                 queue:nil
	                                            usingBlock:^(NSNotification *n) {
	                                              ghostty_app_set_focus(gApp, false);
	                                            }];
	return true;
}

bool hive_nt_init(const char *configPath, bool userConfig) {
	if (NSThread.isMainThread) return initOnMain(configPath, userConfig);
	__block bool ok = false;
	dispatch_sync(dispatch_get_main_queue(), ^{
	  ok = initOnMain(configPath, userConfig);
	});
	return ok;
}

void hive_nt_reload_config(const char *configPath) {
	NSString *path = configPath ? @(configPath) : nil;
	dispatch_async(dispatch_get_main_queue(), ^{
	  if (!gApp) return;
	  ghostty_config_t cfg = ghostty_config_new();
	  ghostty_config_load_default_files(cfg);
	  if (path) ghostty_config_load_file(cfg, path.UTF8String);
	  ghostty_config_finalize(cfg);
	  ghostty_app_update_config(gApp, cfg);
	  // Surfaces may still read the config they were given, so the previous
	  // one is freed only once this one replaces it, as Ghostty's own app does.
	  ghostty_config_free(gConfig);
	  gConfig = cfg;
	});
}

static NSView *webviewOf(NSWindow *window) {
	for (NSView *v in window.contentView.subviews) {
		if ([v isKindOfClass:WKWebView.class]) return v;
	}
	return window.contentView;
}

void hive_nt_create(uintptr_t handle, void *nsWindow, const char *command, const char *cwd, double fontSize,
                    const char **envKeys, const char **envValues, int envCount) {
	NSString *cmd = command ? @(command) : nil;
	NSString *dir = cwd ? @(cwd) : nil;
	NSMutableArray<NSString *> *keys = [NSMutableArray array], *values = [NSMutableArray array];
	for (int i = 0; i < envCount; i++) {
		[keys addObject:@(envKeys[i])];
		[values addObject:@(envValues[i])];
	}
	NSWindow *window = (__bridge NSWindow *)nsWindow;
	dispatch_async(dispatch_get_main_queue(), ^{
	  if (!gApp || gViews[@(handle)]) return;
	  NSView *web = webviewOf(window);
	  HiveGhosttyView *view = [[HiveGhosttyView alloc] initWithFrame:NSMakeRect(0, 0, 10, 10) handle:handle];
	  view.hidden = YES;
	  [window.contentView addSubview:view positioned:NSWindowAbove relativeTo:web];

	  ghostty_surface_config_s cfg = ghostty_surface_config_new();
	  cfg.platform_tag = GHOSTTY_PLATFORM_MACOS;
	  cfg.platform.macos.nsview = (__bridge void *)view;
	  cfg.userdata = (__bridge void *)view;
	  cfg.scale_factor = window.backingScaleFactor;
	  cfg.font_size = (float)fontSize;
	  cfg.command = cmd.UTF8String;
	  cfg.working_directory = dir.UTF8String;
	  cfg.wait_after_command = false;
	  cfg.context = GHOSTTY_SURFACE_CONTEXT_WINDOW;
	  ghostty_env_var_s env[keys.count ?: 1];
	  for (NSUInteger i = 0; i < keys.count; i++) env[i] = (ghostty_env_var_s){keys[i].UTF8String, values[i].UTF8String};
	  cfg.env_vars = keys.count ? env : NULL;
	  cfg.env_var_count = keys.count;
	  view.surface = ghostty_surface_new(gApp, &cfg);
	  if (!view.surface) {
		  [view removeFromSuperview];
		  emit(handle, HIVE_NT_CLOSED, @"surface failed to start");
		  return;
	  }
	  gViews[@(handle)] = view;
	  [view syncDisplay];
	  [view viewDidChangeBackingProperties];
	});
}

void hive_nt_set_frame(uintptr_t handle, double x, double y, double w, double h, bool visible) {
	dispatch_async(dispatch_get_main_queue(), ^{
	  HiveGhosttyView *view = (HiveGhosttyView *)gViews[@(handle)];
	  if (!view) return;
	  NSView *web = webviewOf(view.window);
	  // The webview is flipped (top-left origin, CSS pixels at zoom 1); its
	  // superview is not.
	  NSRect inWeb = NSMakeRect(x, y, w, h);
	  NSRect frame = [web convertRect:inWeb toView:view.superview];
	  [CATransaction begin];
	  [CATransaction setDisableActions:YES];
	  view.frame = NSIntegralRect(frame);
	  [view applyMask];
	  view.hidden = !visible || w < 1 || h < 1;
	  [CATransaction commit];
	  if (view.surface) ghostty_surface_set_occlusion(view.surface, !view.hidden);
	  if (view.hidden && view.window.firstResponder == view) [view.window makeFirstResponder:web];
	});
}

void hive_nt_set_occluders(const double *rects, int count) {
	NSMutableArray<NSValue *> *list = [NSMutableArray arrayWithCapacity:count];
	for (int i = 0; i < count; i++) {
		const double *r = rects + i * 4;
		[list addObject:[NSValue valueWithRect:NSMakeRect(r[0], r[1], r[2], r[3])]];
	}
	dispatch_async(dispatch_get_main_queue(), ^{
	  gOccluders = list;
	  for (HiveGhosttyView *view in gViews.allValues) [view applyMask];
	});
}

void hive_nt_focus(uintptr_t handle) {
	dispatch_async(dispatch_get_main_queue(), ^{
	  HiveGhosttyView *view = (HiveGhosttyView *)gViews[@(handle)];
	  if (view && !view.hidden) [view.window makeFirstResponder:view];
	});
}

void hive_nt_destroy(uintptr_t handle) {
	dispatch_async(dispatch_get_main_queue(), ^{
	  HiveGhosttyView *view = (HiveGhosttyView *)gViews[@(handle)];
	  if (!view) return;
	  [gViews removeObjectForKey:@(handle)];
	  if (view.window.firstResponder == view) [view.window makeFirstResponder:webviewOf(view.window)];
	  ghostty_surface_t s = view.surface;
	  view.surface = NULL;
	  [view removeFromSuperview];
	  if (s) ghostty_surface_free(s);
	});
}

