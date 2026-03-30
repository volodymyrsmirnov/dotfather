package shellinit

import "fmt"

const bashZshInit = `# dotfather shell integration
dotfather() {
    if [ "$1" = "cd" ]; then
        local dir
        dir="$(command dotfather cd)" || return
        cd "$dir" || return
    else
        command dotfather "$@"
    fi
}
`

const fishInit = `# dotfather shell integration
function dotfather
    if test "$argv[1]" = "cd"
        set -l dir (command dotfather cd); or return
        cd $dir; or return
    else
        command dotfather $argv
    end
end
`

// ForShell returns the shell init script for the given shell name.
func ForShell(shell string) (string, error) {
	switch shell {
	case "bash", "zsh":
		return bashZshInit, nil
	case "fish":
		return fishInit, nil
	default:
		return "", fmt.Errorf("unsupported shell: %s (supported: bash, zsh, fish)", shell)
	}
}
