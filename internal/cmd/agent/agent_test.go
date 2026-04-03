package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

type stubAgentService struct {
	name    string
	nameErr error
}

func (s stubAgentService) AgentName(context.Context) (string, error) {
	return s.name, s.nameErr
}

func TestNewNameCmd_TextOutput(t *testing.T) {
	// Given
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}
	original := newAgentService
	newAgentService = func(_ *cmdutil.Factory) (agentService, error) {
		return stubAgentService{name: "blue-seal-echo"}, nil
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
		return stubAgentService{name: "blue-seal-echo"}, nil
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
		return stubAgentService{nameErr: wantErr}, nil
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
