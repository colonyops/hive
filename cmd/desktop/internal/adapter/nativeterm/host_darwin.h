//go:build darwin && ghostty && !server

#ifndef HIVE_NATIVETERM_H
#define HIVE_NATIVETERM_H

#include <stdbool.h>
#include <stdint.h>

// Event kinds sent to Go through hiveNativeTermEvent.
enum {
	HIVE_NT_CLOSED = 1,   // the surface's command ended
	HIVE_NT_TITLE = 2,    // data: the new title
	HIVE_NT_FOCUS = 3,    // data: "1" or "0"
	HIVE_NT_KEY = 4,      // data: a JSON key chord the web keymap may claim
	HIVE_NT_OPEN_URL = 5, // data: the URL
	HIVE_NT_CELL = 6,     // data: "width,height" of a cell in points
};

// hive_nt_init loads libghostty: the user's own Ghostty config when
// userConfig, then the file at configPath (may be NULL) on top. Returns false
// on failure.
bool hive_nt_init(const char *configPath, bool userConfig);

// hive_nt_create adds a surface view running command in cwd to the window
// that owns nsWindow, above its webview. Asynchronous: runs on the main
// thread.
void hive_nt_create(uintptr_t handle, void *nsWindow, const char *command, const char *cwd, double fontSize,
                    const char **envKeys, const char **envValues, int envCount);

// hive_nt_reload_config reloads the user's Ghostty config and the file at
// configPath on top, into the app and every open surface.
void hive_nt_reload_config(const char *configPath);

// hive_nt_set_frame places the view over the webview rectangle (CSS pixels,
// origin top-left of the webview) and shows or hides it.
void hive_nt_set_frame(uintptr_t handle, double x, double y, double w, double h, bool visible);

// hive_nt_set_occluders sets the page elements floating over the terminals,
// as count rectangles of x, y, w, h in CSS pixels from the webview's top-left.
// Every surface cuts them out and lets clicks on them through.
void hive_nt_set_occluders(const double *rects, int count);

void hive_nt_focus(uintptr_t handle);
void hive_nt_destroy(uintptr_t handle);


#endif
