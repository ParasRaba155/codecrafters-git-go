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
		initCMD()
	case "cat-file":
		catFileCmd()
	case "hash-object":
		hashObjectCmd()
	case "ls-tree":
		lsTreeCmd()
	case "write-tree":
		writeTreeCmd()
	case "commit-tree":
		commitTreeCmd()
	default:
		ePrintf("Unknown command %s\n", command)
		os.Exit(1)
	}
}

func ePrintf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format, a...)
}
