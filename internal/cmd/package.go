package cmd

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/goreleaser/nfpm/v2"
	"github.com/muesli/coral"
)

type packageCmd struct {
	cmd      *coral.Command
	config   string
	target   string
	packager string
}

func newPackageCmd() *packageCmd {
	root := &packageCmd{}
	cmd := &coral.Command{
		Use:           "package",
		Aliases:       []string{"pkg", "p"},
		Short:         "Creates a package based on the given the given config file and flags",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          coral.NoArgs,
		RunE: func(cmd *coral.Command, args []string) error {
			return doPackage(root.config, root.target, root.packager)
		},
	}

	cmd.Flags().StringVarP(&root.config, "config", "f", "nfpm.yaml", "config file to be used")
	cmd.Flags().StringVarP(&root.target, "target", "t", "", "where to save the generated package (filename, folder or empty for current folder)")
	cmd.Flags().StringVarP(&root.packager, "packager", "p", "", "which packager implementation to use [apk|deb|rpm]")

	root.cmd = cmd
	return root
}

var errInsufficientParams = errors.New("a packager must be specified if target is a directory or blank")

// nolint:funlen
func doPackage(configPath, target, packager string) error {
	targetIsADirectory := false
	stat, err := os.Stat(target)
	if err == nil && stat.IsDir() {
		targetIsADirectory = true
	}

	if packager == "" {
		ext := filepath.Ext(target)
		if targetIsADirectory || ext == "" {
			return errInsufficientParams
		}

		packager = ext[1:]
		fmt.Println("guessing packager from target file extension...")
	}

	config, err := nfpm.ParseFile(configPath)
	if err != nil {
		return err
	}

	info, err := config.Get(packager)
	if err != nil {
		return err
	}

	info = nfpm.WithDefaults(info)

	fmt.Printf("using %s packager...\n", packager)
	pkg, err := nfpm.Get(packager)
	if err != nil {
		return err
	}

	if target == "" {
		// if no target was specified create a package in
		// current directory with a conventional file name
		target = pkg.ConventionalFileName(info)
	} else if targetIsADirectory {
		// if a directory was specified as target, create
		// a package with conventional file name there
		target = path.Join(target, pkg.ConventionalFileName(info))
	}

	f, err := os.Create(target)
	if err != nil {
		return err
	}

	info.Target = target

	err = pkg.Package(info, f)
	_ = f.Close()
	if err != nil {
		os.Remove(target)
		return err
	}

	fmt.Printf("created package: %s\n", target)
	return nil
}
