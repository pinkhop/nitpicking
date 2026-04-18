// Package completion provides the "completion" command, which outputs shell
// completion scripts for bash, zsh, and fish.
package completion

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// NewCmd constructs the "completion" command which outputs shell completion
// scripts. Users source the output in their shell profile to enable tab
// completion for np subcommands and flags.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:      "completion",
		Usage:     "Output shell completion script",
		ArgsUsage: "<shell>  where <shell> is: bash, zsh, fish",
		Description: `Outputs a shell completion script that enables tab-completion for np
commands, subcommands, and flags. Source the output in your shell
profile so completions are available in every new session.

Supported shells are bash, zsh, and fish. Pass the shell name as the
first positional argument. For example:

  eval "$(np admin completion bash)"   # add to ~/.bashrc
  eval "$(np admin completion zsh)"    # add to ~/.zshrc
  np admin completion fish | source    # add to fish config

The script provides basic subcommand completion. It does not complete
issue IDs or dynamic values — those require the full database, which
shell completion scripts cannot access efficiently.`,
		Action: func(_ context.Context, cmd *cli.Command) error {
			shell := cmd.Args().Get(0)
			if shell == "" {
				return cmdutil.FlagErrorf("shell argument is required: bash, zsh, or fish")
			}

			var script string
			switch shell {
			case "bash":
				script = bashCompletion
			case "zsh":
				script = zshCompletion
			case "fish":
				script = fishCompletion
			default:
				return cmdutil.FlagErrorf("unsupported shell %q: must be bash, zsh, or fish", shell)
			}

			w := io.Writer(os.Stdout)
			if f != nil && f.IOStreams != nil && f.IOStreams.Out != nil {
				w = f.IOStreams.Out
			}

			_, err := fmt.Fprint(w, script)
			return err
		},
	}
}

// bashCompletion is the bash completion script for np. It provides basic
// subcommand completion using the _np function.
const bashCompletion = `# bash completion for np
# Add to ~/.bashrc: eval "$(np admin completion bash)"

_np() {
    local cur prev words cword
    _init_completion || return

    local commands="issue epic claim create done ready blocked show list history comment rel label admin agent graph import init version"

    if [[ ${cword} -eq 1 ]]; then
        COMPREPLY=($(compgen -W "${commands}" -- "${cur}"))
        return
    fi

    case "${words[1]}" in
        issue)
            COMPREPLY=($(compgen -W "list search update edit close release reopen defer delete comment orphans" -- "${cur}"))
            ;;
        epic)
            COMPREPLY=($(compgen -W "status close-completed children" -- "${cur}"))
            ;;
        rel)
            COMPREPLY=($(compgen -W "add blocks refs parent list tree graph" -- "${cur}"))
            ;;
        comment)
            COMPREPLY=($(compgen -W "add list" -- "${cur}"))
            ;;
        label|l)
            COMPREPLY=($(compgen -W "add remove list list-all propagate" -- "${cur}"))
            ;;
        admin)
            COMPREPLY=($(compgen -W "backup completion doctor gc reset restore tally upgrade where" -- "${cur}"))
            ;;
        agent)
            COMPREPLY=($(compgen -W "name prime" -- "${cur}"))
            ;;
        claim)
            COMPREPLY=($(compgen -W "id ready" -- "${cur}"))
            ;;
    esac
}

complete -F _np np
`

// zshCompletion is the zsh completion script for np. It uses the compctl
// function for basic subcommand completion.
const zshCompletion = `# zsh completion for np
# Add to ~/.zshrc: eval "$(np admin completion zsh)"

_np() {
    local -a commands
    commands=(
        'issue:Issue management commands'
        'epic:Epic-specific operations'
        'claim:Claim issues (by ID or next ready)'
        'create:Create a new issue'
        'done:Close an issue that you have claimed'
        'ready:List the open issues with no blockers'
        'blocked:List open and deferred issues that are blocked'
        'show:Show issue detail'
        'list:List issues'
        'history:Show issue history'
        'comment:Manage issue comments'
        'rel:Manage relationships between issues'
        'label:Manage issue labels'
        'admin:Administrative and maintenance commands'
        'agent:Agent utilities'
        'graph:Generate Graphviz DOT graph'
        'import:Import issues from a JSONL file'
        'init:Initialize a new np database'
        'version:Show version information'
    )

    _describe -t commands 'np commands' commands
}

compdef _np np
`

// fishCompletion is the fish completion script for np.
const fishCompletion = `# fish completion for np
# Add to ~/.config/fish/completions/np.fish or eval: np admin completion fish | source

# Disable file completions by default.
complete -c np -f

# Top-level commands.
complete -c np -n "__fish_use_subcommand" -a "issue" -d "Issue management commands"
complete -c np -n "__fish_use_subcommand" -a "epic" -d "Epic-specific operations"
complete -c np -n "__fish_use_subcommand" -a "claim" -d "Claim issues (by ID or next ready)"
complete -c np -n "__fish_use_subcommand" -a "create" -d "Create a new issue"
complete -c np -n "__fish_use_subcommand" -a "close" -d "Close an issue that you have claimed"
complete -c np -n "__fish_use_subcommand" -a "ready" -d "List the open issues with no blockers"
complete -c np -n "__fish_use_subcommand" -a "blocked" -d "List open and deferred issues that are blocked"
complete -c np -n "__fish_use_subcommand" -a "show" -d "Show issue detail"
complete -c np -n "__fish_use_subcommand" -a "list" -d "List issues"
complete -c np -n "__fish_use_subcommand" -a "history" -d "Show issue history"
complete -c np -n "__fish_use_subcommand" -a "comment" -d "Manage issue comments"
complete -c np -n "__fish_use_subcommand" -a "rel" -d "Manage relationships"
complete -c np -n "__fish_use_subcommand" -a "label" -d "Manage issue labels"
complete -c np -n "__fish_use_subcommand" -a "admin" -d "Administrative and maintenance commands"
complete -c np -n "__fish_use_subcommand" -a "agent" -d "Agent utilities"
complete -c np -n "__fish_use_subcommand" -a "graph" -d "Generate Graphviz DOT graph"
complete -c np -n "__fish_use_subcommand" -a "import" -d "Import issues from a JSONL file"

# issue subcommands.
complete -c np -n "__fish_seen_subcommand_from issue" -a "list search update edit close release reopen defer delete comment orphans"

# epic subcommands.
complete -c np -n "__fish_seen_subcommand_from epic" -a "status close-completed children"

# rel subcommands.
complete -c np -n "__fish_seen_subcommand_from rel" -a "add blocks refs parent list tree graph"

# comment subcommands.
complete -c np -n "__fish_seen_subcommand_from comment" -a "add list"

# label subcommands.
complete -c np -n "__fish_seen_subcommand_from label" -a "add remove list list-all propagate"

# admin subcommands.
complete -c np -n "__fish_seen_subcommand_from admin" -a "backup completion doctor gc reset restore tally upgrade where"

# claim subcommands.
complete -c np -n "__fish_seen_subcommand_from claim" -a "id ready"
`
