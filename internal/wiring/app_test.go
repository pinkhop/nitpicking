package wiring

import (
	"runtime"
	"testing"
)

// ---------------------------------------------------------------------------
// AppNameFromArgs
// ---------------------------------------------------------------------------

func TestAppNameFromArgs(t *testing.T) {
	t.Parallel()

	// windowsPathWant reflects filepath.Base's intentional OS-aware behavior:
	// on Windows, backslash is a separator so the directory is stripped and
	// the result is "myapp"; on Unix, backslash is a valid filename character
	// so filepath.Base returns the whole string and only .exe is trimmed.
	windowsPathWant := `C:\bin\myapp`
	if runtime.GOOS == "windows" {
		windowsPathWant = "myapp"
	}

	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "UnixAbsolutePath_StripsDirComponent",
			args: []string{"/usr/local/bin/myapp"},
			want: "myapp",
		},
		{
			name: "WindowsPathWithExe_BehaviorIsOSDependent",
			args: []string{`C:\bin\myapp.exe`},
			want: windowsPathWant,
		},
		{
			name: "BareNameWithExeSuffix_StripsExe",
			args: []string{"myapp.exe"},
			want: "myapp",
		},
		{
			name: "BareName_NoDirectory_ReturnedAsIs",
			args: []string{"myapp"},
			want: "myapp",
		},
		{
			name: "EmptyArgs_ReturnsDefaultFallback",
			args: []string{},
			want: "app",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got := AppNameFromArgs(tc.args)

			// Then
			if got != tc.want {
				t.Errorf("AppNameFromArgs(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// NewCore
// ---------------------------------------------------------------------------

func TestNewCore_ReturnsAppNameAndVersion(t *testing.T) {
	t.Parallel()

	// Given
	const appName = "example-app"

	// When
	f := NewCore(appName)
	actualName := f.AppName
	actualVer := f.AppVersion

	// Then
	if actualName != appName {
		t.Errorf("expected app name %q, got %q", appName, actualName)
	}
	// NewCore uses the package-level version variable, which defaults to "dev"
	// in tests (no ldflags injection).
	if actualVer != "dev" {
		t.Errorf("expected app version %q, got %q", "dev", actualVer)
	}
}
