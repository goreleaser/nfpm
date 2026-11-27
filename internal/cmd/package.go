package cmd

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	"github.com/spf13/cobra"
)

type packageCmd struct {
	cmd      *cobra.Command
	config   string
	target   string
	packager string
}

func newPackageCmd() *packageCmd {
	root := &packageCmd{}
	cmd := &cobra.Command{
		Use:               "package",
		Aliases:           []string{"pkg", "p"},
		Short:             "Creates a package based on the given config file and flags",
		SilenceUsage:      true,
		SilenceErrors:     true,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(*cobra.Command, []string) error {
			return doPackage(root.config, root.target, root.packager)
		},
	}

	cmd.Flags().StringVarP(&root.config, "config", "f", "nfpm.yaml", "config file to be used")
	_ = cmd.MarkFlagFilename("config", "yaml", "yml")
	cmd.Flags().StringVarP(&root.target, "target", "t", "", "where to save the generated package (filename, folder or empty for current folder)")
	_ = cmd.MarkFlagFilename("target")

	pkgs := nfpm.Enumerate()

	cmd.Flags().StringVarP(&root.packager, "packager", "p", "",
		fmt.Sprintf("which packager implementation to use [%s]", strings.Join(pkgs, "|")))
	_ = cmd.RegisterFlagCompletionFunc("packager", cobra.FixedCompletions(
		pkgs,
		cobra.ShellCompDirectiveNoFileComp,
	))

	root.cmd = cmd
	return root
}

var errInsufficientParams = errors.New("a packager must be specified if target is a directory or blank")

// extensionMismatch reports the packager whose conventional extension target
// ends with, when that packager is not the one requested.
//
// Packager names are not file extensions: srpm writes .src.rpm, which
// filepath.Ext reports as .rpm, and archlinux writes .pkg.tar.zst, which it
// reports as .zst. Matching the conventional extensions longest first keeps
// srpm packages from being rejected as rpm, and still catches a .pkg.tar.zst
// target requested as deb.
func extensionMismatch(target, packager string) (string, string) {
	target = strings.ToLower(target)

	var matchedExt, matchedPackager string
	for _, name := range nfpm.Enumerate() {
		pkg, err := nfpm.Get(name)
		if err != nil {
			continue
		}
		withExt, ok := pkg.(nfpm.PackagerWithExtension)
		if !ok {
			continue
		}
		ext := strings.ToLower(withExt.ConventionalExtension())
		if ext == "" || !strings.HasSuffix(target, ext) {
			continue
		}
		if len(ext) > len(matchedExt) {
			matchedExt, matchedPackager = ext, name
		}
	}

	if matchedExt == "" || matchedPackager == packager {
		return "", ""
	}
	return matchedExt, matchedPackager
}

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

	if target != "" && !targetIsADirectory {
		if ext, other := extensionMismatch(target, packager); ext != "" {
			return fmt.Errorf(
				"target %q has the extension %q of the %s packager, but %q was requested",
				target, ext, other, packager,
			)
		}
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
	defer f.Close()

	info.Target = target

	if err := pkg.Package(info, f); err != nil {
		os.Remove(target)
		return err
	}

	fmt.Printf("created package: %s\n", target)
	return f.Close()
}
