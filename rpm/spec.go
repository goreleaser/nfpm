package rpm

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
)

// generateSpec synthesizes a self-contained, rebuildable RPM spec from the
// package metadata. %install extracts the bundled source tarball into the
// buildroot, and %files re-declares every path with its attributes, so
// `rpmbuild --rebuild` reproduces the binary RPM.
func generateSpec(info *nfpm.Info, sourceName string) (string, error) {
	summary := defaultTo(info.RPM.Summary, strings.Split(info.Description, "\n")[0])
	summary = defaultTo(summary, info.Name)
	description := defaultTo(info.Description, summary)

	var b strings.Builder

	// Ship prebuilt artifacts verbatim: no debuginfo extraction, no binary
	// post-processing/stripping, and no auto-generated dependencies.
	b.WriteString("%global debug_package %{nil}\n")
	b.WriteString("%global __os_install_post %{nil}\n")
	b.WriteString("%define _build_id_links none\n\n")

	writeField := func(key, value string) {
		if value != "" {
			fmt.Fprintf(&b, "%s: %s\n", key, value)
		}
	}

	writeField("Name", info.Name)
	writeField("Version", formatVersion(info))
	writeField("Release", defaultTo(info.Release, "1"))
	if info.Epoch != "" {
		writeField("Epoch", info.Epoch)
	}
	writeField("Summary", summary)
	writeField("License", info.License)
	writeField("URL", info.Homepage)
	writeField("Group", info.RPM.Group)
	writeField("Vendor", info.Vendor)
	writeField("Packager", defaultTo(info.RPM.Packager, info.Maintainer))
	writeField("BuildArch", info.Arch)
	for _, prefix := range info.RPM.Prefixes {
		writeField("Prefix", prefix)
	}

	// Dependencies are declared explicitly; do not let rpmbuild scan the prebuilt
	// payload for additional ones.
	b.WriteString("AutoReqProv: no\n")
	for _, dep := range info.Depends {
		writeField("Requires", dep)
	}
	for _, dep := range info.RPM.Requires.Post {
		writeField("Requires(post)", dep)
	}
	for _, dep := range info.Provides {
		writeField("Provides", dep)
	}
	for _, dep := range info.Conflicts {
		writeField("Conflicts", dep)
	}
	for _, dep := range info.Replaces {
		writeField("Obsoletes", dep)
	}
	for _, dep := range info.Recommends {
		writeField("Recommends", dep)
	}
	for _, dep := range info.Suggests {
		writeField("Suggests", dep)
	}

	fmt.Fprintf(&b, "Source0: %s\n", sourceName)

	fmt.Fprintf(&b, "\n%%description\n%s\n", description)

	b.WriteString("\n%prep\n")
	b.WriteString("\n%build\n")

	b.WriteString("\n%install\n")
	b.WriteString("rm -rf %{buildroot}\n")
	b.WriteString("mkdir -p %{buildroot}\n")
	b.WriteString("tar -C %{buildroot} -xf %{SOURCE0}\n")

	scripts, err := readScripts(info)
	if err != nil {
		return "", err
	}
	writeScriptSection(&b, "pre", scripts.preIn)
	writeScriptSection(&b, "post", scripts.postIn)
	writeScriptSection(&b, "preun", scripts.preUn)
	writeScriptSection(&b, "postun", scripts.postUn)
	writeScriptSection(&b, "pretrans", scripts.preTrans)
	writeScriptSection(&b, "posttrans", scripts.postTrans)
	writeScriptSection(&b, "verifyscript", scripts.verify)

	b.WriteString("\n%files\n")
	writeFilesSection(&b, info)

	changelog, err := renderSpecChangelog(info)
	if err != nil {
		return "", err
	}
	b.WriteString(changelog)

	return b.String(), nil
}

func writeScriptSection(b *strings.Builder, section, body string) {
	if body == "" {
		return
	}
	fmt.Fprintf(b, "\n%%%s\n%s", section, body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
}

func writeFilesSection(b *strings.Builder, info *nfpm.Info) {
	for _, content := range info.Contents {
		if content.Packager != "" && content.Packager != contentPackager {
			continue
		}
		if line := specFileLine(content); line != "" {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
}

func specFileLine(content *files.Content) string {
	dest := files.ToNixPath(content.Destination)
	owner := defaultTo(content.FileInfo.Owner, "root")
	group := defaultTo(content.FileInfo.Group, "root")

	switch content.Type {
	case files.TypeImplicitDir:
		return ""
	case files.TypeSymlink:
		return fmt.Sprintf("%%attr(-, %s, %s) %q", owner, group, dest)
	case files.TypeDir:
		return fmt.Sprintf("%%dir %s", attrPath(content.Mode(), owner, group, dest))
	case files.TypeConfig:
		return "%config " + attrPath(content.FileInfo.Mode, owner, group, dest)
	case files.TypeConfigNoReplace:
		return "%config(noreplace) " + attrPath(content.FileInfo.Mode, owner, group, dest)
	case files.TypeConfigMissingOK:
		return "%config(missingok) " + attrPath(content.FileInfo.Mode, owner, group, dest)
	case files.TypeRPMDoc:
		return "%doc " + attrPath(content.FileInfo.Mode, owner, group, dest)
	case files.TypeRPMLicence, files.TypeRPMLicense:
		return "%license " + attrPath(content.FileInfo.Mode, owner, group, dest)
	case files.TypeRPMReadme:
		return "%doc " + attrPath(content.FileInfo.Mode, owner, group, dest)
	case files.TypeRPMGhost:
		mode := content.FileInfo.Mode
		if mode == 0 {
			mode = 0o644
		}
		return "%ghost " + attrPath(mode, owner, group, dest)
	default:
		return attrPath(content.FileInfo.Mode, owner, group, dest)
	}
}

// attrPath renders an "%attr(mode, owner, group) path" entry for the %files
// section, normalizing the mode to its octal RPM representation.
func attrPath(mode fs.FileMode, owner, group, dest string) string {
	return fmt.Sprintf("%%attr(%04o, %s, %s) %q", normalizeFileMode(mode), owner, group, dest)
}
