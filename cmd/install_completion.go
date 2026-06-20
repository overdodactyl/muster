package cmd

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"muster/internal/render"
)

var installCompletionForce bool

var installCompletionCmd = &cobra.Command{
	Use:   "install-completion [bash|zsh|fish]",
	Short: "Install shell completion to the conventional location for your shell",
	Long: `Detects the user's shell (from $SHELL or the positional argument) and
writes muster's completion script to the conventional location:

  bash  ~/.local/share/bash-completion/completions/muster
  zsh   ~/.zsh/completions/_muster   (and prints fpath snippet to add)
  fish  ~/.config/fish/completions/muster.fish

After install, restart the shell (or source the file) so completion kicks in.

Use --force to overwrite an existing file.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := ""
		if len(args) > 0 {
			shell = args[0]
		} else {
			shell = filepath.Base(os.Getenv("SHELL"))
		}

		home := os.Getenv("HOME")
		if home == "" {
			if u, err := user.Current(); err == nil {
				home = u.HomeDir
			}
		}
		if home == "" {
			return fmt.Errorf("could not determine $HOME")
		}

		var path string
		var generator func(*os.File) error
		var post string

		switch shell {
		case "bash":
			path = filepath.Join(home, ".local", "share", "bash-completion", "completions", "muster")
			generator = func(f *os.File) error { return rootCmd.GenBashCompletionV2(f, true) }
			post = "ensure '~/.local/share/bash-completion/completions' is in your bash completion path (modern bash + bash-completion picks it up automatically)."
		case "zsh":
			path = filepath.Join(home, ".zsh", "completions", "_muster")
			generator = func(f *os.File) error { return rootCmd.GenZshCompletion(f) }
			post = "add this to your ~/.zshrc if not present:\n  fpath=(~/.zsh/completions $fpath)\n  autoload -U compinit && compinit"
		case "fish":
			path = filepath.Join(home, ".config", "fish", "completions", "muster.fish")
			generator = func(f *os.File) error { return rootCmd.GenFishCompletion(f, true) }
			post = "fish reloads completions automatically on next invocation."
		default:
			return fmt.Errorf("unknown shell %q (want bash, zsh, or fish)", shell)
		}

		if _, err := os.Stat(path); err == nil && !installCompletionForce {
			return fmt.Errorf("%s already exists (use --force to overwrite)", path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		if err := generator(f); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		fmt.Printf("%s %s\n", render.ColorGreen("✓"), path)
		if post != "" {
			fmt.Println(strings.TrimSpace(post))
		}
		return nil
	},
}

func init() {
	installCompletionCmd.Flags().BoolVar(&installCompletionForce, "force", false, "overwrite the file if it already exists")
	rootCmd.AddCommand(installCompletionCmd)
}
