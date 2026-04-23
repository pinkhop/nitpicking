package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

type stubAgentService struct {
	name         string
	nameErr      error
	capturedSeed string
}

// AgentName satisfies the agentService interface. It records the seed from the
// input so tests can assert that the flag was forwarded to the service, then
// returns the preconfigured name and error.
func (s *stubAgentService) AgentName(_ context.Context, input driving.AgentNameInput) (string, error) {
	s.capturedSeed = input.Seed
	return s.name, s.nameErr
}

func TestNewNameCmd_TextOutput(t *testing.T) {
	// Given
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}
	original := newAgentService
	newAgentService = func(_ *cmdutil.Factory) (agentService, error) {
		return &stubAgentService{name: "blue-seal-echo"}, nil
	}
	t.Cleanup(func() { newAgentService = original })

	// When
	err := newNameCmd(f).Run(t.Context(), []string{"name"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout.String() != "blue-seal-echo\n" {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestNewNameCmd_JSONOutput(t *testing.T) {
	// Given
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}
	original := newAgentService
	newAgentService = func(_ *cmdutil.Factory) (agentService, error) {
		return &stubAgentService{name: "blue-seal-echo"}, nil
	}
	t.Cleanup(func() { newAgentService = original })

	// When
	err := newNameCmd(f).Run(t.Context(), []string{"name", "--json"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out nameOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}
	if out.Name != "blue-seal-echo" {
		t.Fatalf("name: got %q, want %q", out.Name, "blue-seal-echo")
	}
}

func TestNewNameCmd_WrapsServiceErrors(t *testing.T) {
	// Given
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}
	wantErr := errors.New("service boom")
	original := newAgentService
	newAgentService = func(_ *cmdutil.Factory) (agentService, error) {
		return &stubAgentService{nameErr: wantErr}, nil
	}
	t.Cleanup(func() { newAgentService = original })

	// When
	err := newNameCmd(f).Run(t.Context(), []string{"name"})

	// Then
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped error to contain original, got %v", err)
	}
	if !strings.Contains(err.Error(), "generating agent name") {
		t.Fatalf("expected wrapped context, got %v", err)
	}
}

func TestNewNameCmd_SeedFlag_ForwardsSeedToService(t *testing.T) {
	// Given
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}
	stub := &stubAgentService{name: "seeded-name-result"}
	original := newAgentService
	newAgentService = func(_ *cmdutil.Factory) (agentService, error) {
		return stub, nil
	}
	t.Cleanup(func() { newAgentService = original })

	// When
	err := newNameCmd(f).Run(t.Context(), []string{"name", "--seed=my-stable-seed"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.capturedSeed != "my-stable-seed" {
		t.Fatalf("seed forwarded to service: got %q, want %q", stub.capturedSeed, "my-stable-seed")
	}
	if stdout.String() != "seeded-name-result\n" {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

// TestNewNameCmd_BlankSeed_IsRejected verifies that --seed values that are
// empty or contain only whitespace are rejected with a FlagError. urfave/cli
// strips trailing whitespace from string flags, so whitespace-only seeds arrive
// with seed=="" and IsSet("seed")==true — the same condition as an explicit
// --seed="" — and must be caught identically.
func TestNewNameCmd_BlankSeed_IsRejected(t *testing.T) {
	cases := []struct {
		name string
		seed string
	}{
		{"empty string", "--seed="},
		{"spaces only", "--seed=   "},
		{"tab only", "--seed=\t"},
		{"mixed whitespace", "--seed=\t\n "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Validation fires before the service is constructed, so no
			// newAgentService swap is needed. Do not call t.Parallel() here
			// because the sibling tests that do swap newAgentService are
			// also not parallel — mixing the two would introduce races.

			// Given
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios}

			// When
			err := newNameCmd(f).Run(t.Context(), []string{"name", tc.seed})

			// Then
			if err == nil {
				t.Fatal("expected validation error for blank seed, got nil")
			}
			var flagErr *cmdutil.FlagError
			if !errors.As(err, &flagErr) {
				t.Fatalf("expected *cmdutil.FlagError, got %T: %v", err, err)
			}
		})
	}
}

func TestNewPrimeCmd_TextOutput(t *testing.T) {
	// Given
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	// When
	err := newPrimeCmd(f).Run(t.Context(), []string{"prime"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := AgentInstructions() + "\n"
	if stdout.String() != want {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestNewPrimeCmd_JSONOutput(t *testing.T) {
	// Given
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	// When
	err := newPrimeCmd(f).Run(t.Context(), []string{"prime", "--json"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out primeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}
	if out.Instructions != AgentInstructions() {
		t.Fatalf("instructions mismatch: got %q", out.Instructions)
	}
}
