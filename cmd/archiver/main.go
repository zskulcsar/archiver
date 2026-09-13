// Command archiver creates encrypted, multi-disc archive images.
package main

import (
	"os"

	"github.com/zskulcsar/archiver/internal/cli"
)

var (
	version  = "dev"
	revision = ""
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr, cli.BuildInfo{
		Version:  version,
		Revision: revision,
	}))
}
