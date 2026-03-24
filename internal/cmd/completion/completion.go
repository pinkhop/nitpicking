// Package completion provides the "completion" command, which outputs shell
// completion scripts for bash, zsh, and fish.
package completion

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// NewCmd constructs the "completion" command which outputs shell completion
// scripts. Users source the output in their shell profile to enable tab
// completion for np subcommands and flags.
func NewCmd(_ *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:      "completion",
		Usage:     "Output shell completion script",
		ArgsUsage: "<shell>  where <shell> is: bash, zsh, fish",
		Category:  "Setup",
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

			fmt.Print(script)
			return nil
		},
	}
}

// bashCompletion is the bash completion script for np. It provides basic
// subcommand completion using the _np function.
const bashCompletion = `# bash completion for np
# Add to ~/.bashrc: eval "$(np completion bash)"

_np() {
    local cur prev words cword
    _init_completion || return

    local commands="issue epic create done ready blocked status show list search history comment rel dimension admin agent graph init quickstart version completion"

    if [[ ${cword} -eq 1 ]]; then
        COMPREPLY=($(compgen -W "${commands}" -- "${cur}"))
        return
    fi

    case "${words[1]}" in
        issue)
            COMPREPLY=($(compgen -W "list query update edit close release reopen defer delete note orphans" -- "${cur}"))
            ;;
        epic)
            COMPREPLY=($(compgen -W "status close-eligible children" -- "${cur}"))
            ;;
        rel)
            COMPREPLY=($(compgen -W "add blocks cites parent list tree cycles" -- "${cur}"))
            ;;
        comment)
            COMPREPLY=($(compgen -W "add list" -- "${cur}"))
            ;;
        dimension|dim)
            COMPREPLY=($(compgen -W "add remove list list-all propagate" -- "${cur}"))
            ;;
        admin)
            COMPREPLY=($(compgen -W "doctor gc reset upgrade" -- "${cur}"))
            ;;
        agent)
            COMPREPLY=($(compgen -W "name prime" -- "${cur}"))
            ;;
    esac
}

complete -F _np np
`

// zshCompletion is the zsh completion script for np. It uses the compctl
// function for basic subcommand completion.
const zshCompletion = `# zsh completion for np
# Add to ~/.zshrc: eval "$(np completion zsh)"

_np() {
    local -a commands
    commands=(
        'issue:Issue management commands'
        'epic:Epic-specific operations'
        'create:Create a new issue'
        'done:Close a claimed issue with a required reason'
        'ready:List ready issues'
        'blocked:List blocked issues'
        'status:Dashboard with counts by state'
        'show:Show issue detail'
        'list:List issues'
        'search:Search issues'
        'history:Show issue history'
        'comment:Manage issue comments'
        'rel:Manage relationships between issues'
        'dimension:Manage issue dimensions'
        'admin:Maintenance commands'
        'agent:Agent utilities'
        'graph:Generate Graphviz DOT graph'
        'init:Initialize a new np database'
        'quickstart:Guided setup'
        'version:Show version information'
        'completion:Output shell completion script'
    )

    _describe -t commands 'np commands' commands
}

compdef _np np
`

// fishCompletion is the fish completion script for np.
const fishCompletion = `# fish completion for np
# Add to ~/.config/fish/completions/np.fish or eval: np completion fish | source

# Disable file completions by default.
complete -c np -f

# Top-level commands.
complete -c np -n "__fish_use_subcommand" -a "issue" -d "Issue management commands"
complete -c np -n "__fish_use_subcommand" -a "epic" -d "Epic-specific operations"
complete -c np -n "__fish_use_subcommand" -a "create" -d "Create a new issue"
complete -c np -n "__fish_use_subcommand" -a "done" -d "Close a claimed issue"
complete -c np -n "__fish_use_subcommand" -a "ready" -d "List ready issues"
complete -c np -n "__fish_use_subcommand" -a "blocked" -d "List blocked issues"
complete -c np -n "__fish_use_subcommand" -a "status" -d "Dashboard with counts by state"
complete -c np -n "__fish_use_subcommand" -a "show" -d "Show issue detail"
complete -c np -n "__fish_use_subcommand" -a "list" -d "List issues"
complete -c np -n "__fish_use_subcommand" -a "search" -d "Search issues"
complete -c np -n "__fish_use_subcommand" -a "history" -d "Show issue history"
complete -c np -n "__fish_use_subcommand" -a "comment" -d "Manage issue comments"
complete -c np -n "__fish_use_subcommand" -a "rel" -d "Manage relationships"
complete -c np -n "__fish_use_subcommand" -a "dimension" -d "Manage issue dimensions"
complete -c np -n "__fish_use_subcommand" -a "admin" -d "Maintenance commands"
complete -c np -n "__fish_use_subcommand" -a "agent" -d "Agent utilities"
complete -c np -n "__fish_use_subcommand" -a "graph" -d "Generate Graphviz DOT graph"
complete -c np -n "__fish_use_subcommand" -a "completion" -d "Output shell completion script"

# issue subcommands.
complete -c np -n "__fish_seen_subcommand_from issue" -a "list query update edit close release reopen defer delete note orphans"

# epic subcommands.
complete -c np -n "__fish_seen_subcommand_from epic" -a "status close-eligible children"

# rel subcommands.
complete -c np -n "__fish_seen_subcommand_from rel" -a "add blocks cites parent list tree cycles"

# comment subcommands.
complete -c np -n "__fish_seen_subcommand_from comment" -a "add list"

# dimension subcommands.
complete -c np -n "__fish_seen_subcommand_from dimension" -a "add remove list list-all propagate"

# admin subcommands.
complete -c np -n "__fish_seen_subcommand_from admin" -a "doctor gc reset upgrade"

# completion shells.
complete -c np -n "__fish_seen_subcommand_from completion" -a "bash zsh fish"
`
