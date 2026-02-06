package variants

import (
	"slices"
	"testing"
	"unicode"

	"pgregory.net/rapid"
)

// isFilesystemSafe checks if a character is safe for filesystem and git branch names.
func isFilesystemSafe(c rune) bool {
	// Disallowed: / \ : * ? " < > | and space
	disallowed := []rune{'/', '\\', ':', '*', '?', '"', '<', '>', '|', ' '}
	if slices.Contains(disallowed, c) {
		return false
	}
	// Also disallow control characters
	if unicode.IsControl(c) {
		return false
	}
	return true
}

// TestPropertySanitizeName uses property-based testing to verify the spec name
// sanitization function produces safe, idempotent results.
func TestPropertySanitizeName(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate an arbitrary string
		name := rapid.String().Draw(rt, "name")
		sanitized := sanitizeSpecName(name)

		// Property 1: Result contains only safe filesystem characters
		for _, c := range sanitized {
			if !isFilesystemSafe(c) {
				rt.Fatalf("unsafe character in sanitized name: %q (char: %c)", sanitized, c)
			}
		}

		// Property 2: No consecutive dashes
		for i := 0; i < len(sanitized)-1; i++ {
			if sanitized[i] == '-' && sanitized[i+1] == '-' {
				rt.Fatalf("consecutive dashes in sanitized name: %q", sanitized)
			}
		}

		// Property 3: No leading or trailing dashes
		if len(sanitized) > 0 {
			if sanitized[0] == '-' {
				rt.Fatalf("leading dash in sanitized name: %q", sanitized)
			}
			if sanitized[len(sanitized)-1] == '-' {
				rt.Fatalf("trailing dash in sanitized name: %q", sanitized)
			}
		}

		// Property 4: Idempotent - sanitizing twice gives same result
		doubleSanitized := sanitizeSpecName(sanitized)
		if sanitized != doubleSanitized {
			rt.Fatalf("not idempotent: first=%q second=%q", sanitized, doubleSanitized)
		}
	})
}

// TestPropertySanitizeName_EmptyInput verifies empty input produces empty output.
func TestPropertySanitizeName_EmptyInput(t *testing.T) {
	result := sanitizeSpecName("")
	if result != "" {
		t.Errorf("sanitizeSpecName(\"\") = %q, want empty string", result)
	}
}

// TestPropertySanitizeName_PreservesAlphanumeric verifies alphanumeric strings are preserved.
func TestPropertySanitizeName_PreservesAlphanumeric(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate string containing only lowercase letters and digits
		name := rapid.StringMatching(`^[a-z0-9]+$`).Draw(rt, "alphanumeric")
		if name == "" {
			return // Skip empty
		}

		sanitized := sanitizeSpecName(name)
		if sanitized != name {
			rt.Fatalf("alphanumeric string was modified: input=%q output=%q", name, sanitized)
		}
	})
}

// TestPropertySanitizeName_DashesPreserved verifies single dashes are preserved (not at edges).
func TestPropertySanitizeName_DashesPreserved(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate string like "abc-def" (letter-dash-letter pattern)
		prefix := rapid.StringMatching(`^[a-z]+$`).Draw(rt, "prefix")
		suffix := rapid.StringMatching(`^[a-z]+$`).Draw(rt, "suffix")
		if prefix == "" || suffix == "" {
			return
		}

		name := prefix + "-" + suffix
		sanitized := sanitizeSpecName(name)
		if sanitized != name {
			rt.Fatalf("valid dash-separated string was modified: input=%q output=%q", name, sanitized)
		}
	})
}

// TestPropertySanitizeName_ReplacesUnsafeChars verifies unsafe characters are replaced.
func TestPropertySanitizeName_ReplacesUnsafeChars(t *testing.T) {
	unsafeChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", " "}

	for _, unsafe := range unsafeChars {
		name := "a" + unsafe + "b"
		sanitized := sanitizeSpecName(name)

		// Should not contain the unsafe character
		for _, c := range sanitized {
			if string(c) == unsafe {
				t.Errorf("sanitizeSpecName(%q) still contains %q: result=%q", name, unsafe, sanitized)
			}
		}

		// Should be "a-b" after sanitization
		if sanitized != "a-b" {
			t.Errorf("sanitizeSpecName(%q) = %q, want %q", name, sanitized, "a-b")
		}
	}
}

// TestPropertySanitizeName_CollapsesMultipleDashes verifies multiple dashes are collapsed.
func TestPropertySanitizeName_CollapsesMultipleDashes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"a--b", "a-b"},
		{"a---b", "a-b"},
		{"a----b", "a-b"},
		{"a/ /b", "a-b"}, // Multiple unsafe chars become multiple dashes, then collapsed
		{"a///b", "a-b"}, // Multiple slashes become multiple dashes, then collapsed
		{"a:*:b", "a-b"}, // Mixed unsafe chars
	}

	for _, tc := range tests {
		sanitized := sanitizeSpecName(tc.input)
		if sanitized != tc.expected {
			t.Errorf("sanitizeSpecName(%q) = %q, want %q", tc.input, sanitized, tc.expected)
		}
	}
}

// TestPropertySanitizeName_TrimsEdgeDashes verifies leading/trailing dashes are removed.
func TestPropertySanitizeName_TrimsEdgeDashes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"-abc", "abc"},
		{"abc-", "abc"},
		{"-abc-", "abc"},
		{"--abc--", "abc"},
		{"/abc/", "abc"}, // Slashes at edges become dashes, then trimmed
		{" abc ", "abc"}, // Spaces at edges become dashes, then trimmed
	}

	for _, tc := range tests {
		sanitized := sanitizeSpecName(tc.input)
		if sanitized != tc.expected {
			t.Errorf("sanitizeSpecName(%q) = %q, want %q", tc.input, sanitized, tc.expected)
		}
	}
}

// TestPropertySanitizeName_AllUnsafe verifies a string of only unsafe chars becomes empty.
func TestPropertySanitizeName_AllUnsafe(t *testing.T) {
	tests := []string{
		"/",
		"///",
		" ",
		"   ",
		":*?",
		"<>|",
		"/ / /",
	}

	for _, tc := range tests {
		sanitized := sanitizeSpecName(tc)
		if sanitized != "" {
			t.Errorf("sanitizeSpecName(%q) = %q, want empty string", tc, sanitized)
		}
	}
}
