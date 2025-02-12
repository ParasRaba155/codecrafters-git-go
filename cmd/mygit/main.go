package main

import (
	"fmt"
	"os"
)

// Usage: your_git.sh <command> <arg1> <arg2> ...
func main() {
	// Uncomment this block to pass the first stage!
	//
	if len(os.Args) < 2 {
		ePrintf("usage: mygit <command> [<args>...]\n")
		os.Exit(1)
	}

	switch command := os.Args[1]; command {
	case "init":
		must(initCMD())
	case "cat-file":
		if len(os.Args) != 4 {
			must(fmt.Errorf("usage: mygit cat-file <flag> <file>"))
		}
		if os.Args[2] != "-p" {
			must(fmt.Errorf("usage: mygit cat-file -p <file>"))
		}
		must(catFileCmd(os.Args[3]))
	case "hash-object":
		if len(os.Args) != 4 {
			must(fmt.Errorf("usage: mygit hash-object <flag> <file>"))
		}
		if os.Args[2] != "-w" {
			must(fmt.Errorf("usage: mygit hash-object -w <file>"))
		}
		must(hashObjectCmd(os.Args[3]))
	case "ls-tree":
		if len(os.Args) != 4 {
			must(fmt.Errorf("usage: mygit ls-tree <flag> <file>"))
		}
		if os.Args[2] != "--name-only" {
			must(fmt.Errorf("usage: mygit cat-file --name-only <tree_sha>"))
		}
		must(lsTreeCmd(os.Args[3]))
	case "write-tree":
		if len(os.Args) != 2 {
			must(fmt.Errorf("usage: mygit write-tree"))
		}
		must(writeTreeCmd())
	case "commit-tree":
		if len(os.Args) != 7 {
			must(fmt.Errorf("usage: mygit commit-tree <tree-sha> -p <commit-sha> -m <msg>"))
		}
		if os.Args[3] != "-p" || os.Args[5] != "-m" {
			must(fmt.Errorf("usage: mygit commit-tree <tree-sha> -p <commit-sha> -m <msg>"))
		}
		must(commitTreeCmd(os.Args[2], os.Args[4], os.Args[6]))
	case "clone":
		if len(os.Args) != 4 {
			must(fmt.Errorf("usage: mygit clone <repo_uri> <some_dir>"))
		}
		must(cloneCmd(os.Args[2], os.Args[3]))
	default:
		must(fmt.Errorf("unknown command: %s", command))
	}
}

func ePrintf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format, a...)
}

func must(err error) {
	if err != nil {
		ePrintf("%s\n", err)
		os.Exit(1)
	}
}
