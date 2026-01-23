package main

import (
	"testing"
)

// TestParseSubcommand tests the subcommand parsing logic.
func TestParseSubcommand(t *testing.T) {
	tests := map[string]struct {
		args        []string
		wantCmd     string
		wantCmdArgs []string
	}{
		"no arguments defaults to run": {
			args:        []string{},
			wantCmd:     "run",
			wantCmdArgs: []string{},
		},
		"flag argument defaults to run": {
			args:        []string{"--tasks-file", "foo.md"},
			wantCmd:     "run",
			wantCmdArgs: []string{"--tasks-file", "foo.md"},
		},
		"short flag defaults to run": {
			args:        []string{"-v"},
			wantCmd:     "run",
			wantCmdArgs: []string{"-v"},
		},
		"explicit run command": {
			args:        []string{"run", "--tasks-file", "foo.md"},
			wantCmd:     "run",
			wantCmdArgs: []string{"--tasks-file", "foo.md"},
		},
		"serve command": {
			args:        []string{"serve", "--port", "9000"},
			wantCmd:     "serve",
			wantCmdArgs: []string{"--port", "9000"},
		},
		"serve command no args": {
			args:        []string{"serve"},
			wantCmd:     "serve",
			wantCmdArgs: []string{},
		},
		"register command": {
			args:        []string{"register", "./logs"},
			wantCmd:     "register",
			wantCmdArgs: []string{"./logs"},
		},
		"register command with name flag": {
			args:        []string{"register", "--name", "my-run", "./logs"},
			wantCmd:     "register",
			wantCmdArgs: []string{"--name", "my-run", "./logs"},
		},
		"demo command": {
			args:        []string{"demo"},
			wantCmd:     "demo",
			wantCmdArgs: []string{},
		},
		"consolidate command": {
			args:        []string{"consolidate", "my-feature", "--variant", "1"},
			wantCmd:     "consolidate",
			wantCmdArgs: []string{"my-feature", "--variant", "1"},
		},
		"consolidate command with rollback": {
			args:        []string{"consolidate", "--rollback", "my-feature"},
			wantCmd:     "consolidate",
			wantCmdArgs: []string{"--rollback", "my-feature"},
		},
		"unknown subcommand defaults to run": {
			args:        []string{"unknown-thing"},
			wantCmd:     "run",
			wantCmdArgs: []string{"unknown-thing"},
		},
		"version flag defaults to run": {
			args:        []string{"--version"},
			wantCmd:     "run",
			wantCmdArgs: []string{"--version"},
		},
		"help flag defaults to run": {
			args:        []string{"--help"},
			wantCmd:     "run",
			wantCmdArgs: []string{"--help"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotCmd, gotArgs := parseSubcommand(tc.args)
			if gotCmd != tc.wantCmd {
				t.Errorf("command: got %q, want %q", gotCmd, tc.wantCmd)
			}
			if len(gotArgs) != len(tc.wantCmdArgs) {
				t.Errorf("args length: got %d, want %d", len(gotArgs), len(tc.wantCmdArgs))
				return
			}
			for i, arg := range gotArgs {
				if arg != tc.wantCmdArgs[i] {
					t.Errorf("args[%d]: got %q, want %q", i, arg, tc.wantCmdArgs[i])
				}
			}
		})
	}
}

// TestIsKnownSubcommand tests subcommand recognition.
func TestIsKnownSubcommand(t *testing.T) {
	tests := map[string]struct {
		arg  string
		want bool
	}{
		"run":         {arg: "run", want: true},
		"serve":       {arg: "serve", want: true},
		"register":    {arg: "register", want: true},
		"demo":        {arg: "demo", want: true},
		"consolidate": {arg: "consolidate", want: true},
		"unknown":     {arg: "unknown", want: false},
		"empty":       {arg: "", want: false},
		"flag":        {arg: "--help", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := isKnownSubcommand(tc.arg)
			if got != tc.want {
				t.Errorf("isKnownSubcommand(%q) = %v, want %v", tc.arg, got, tc.want)
			}
		})
	}
}
