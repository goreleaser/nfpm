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
	b.WriteString("%define _build_id_links none\n")
	// Only the files below rpm.doc_dirs are %doc, as in the binary package.
	b.WriteString("%global __docdir_path %{nil}\n\n")

	writeField := func(key, value string) {
		if value != "" {
			fmt.Fprintf(&b, "%s: %s\n", key, escapeSpecText(value))
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
	interpreters, err := scriptRequires(info)
	if err != nil {
		return "", err
	}
	for _, interpreter := range interpreters {
		writeField("Requires", interpreter)
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

	fmt.Fprintf(&b, "\n%%description\n%s\n", escapeSpecText(description))

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
	body = escapeSpecText(body)
	fmt.Fprintf(b, "\n%%%s\n%s", section, body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
}

func writeFilesSection(b *strings.Builder, info *nfpm.Info) {
	docDirs := cleanDocDirs(info.RPM.DocDirs)
	for _, content := range info.Contents {
		if content.Packager != "" && content.Packager != contentPackager {
			continue
		}
		if line := specFileLine(content, docDirs); line != "" {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
}

func specFileLine(content *files.Content, docDirs []string) string {
	dest := files.ToNixPath(content.Destination)
	doc := ""
	if isBelowDocDir(dest, docDirs) {
		doc = "%doc "
	}
	dest = escapeSpecText(dest)
	owner := defaultTo(content.FileInfo.Owner, "root")
	group := defaultTo(content.FileInfo.Group, "root")

	// %lang() is orthogonal to the file directive and prefixes the file-bearing
	// entries (config/doc/license/readme/generic). It is meaningless for
	// directories, symlinks and ghosts, which never carry a language tag.
	langPrefix := ""
	if content.FileInfo.Lang != "" {
		langPrefix = fmt.Sprintf("%%lang(%s) ", content.FileInfo.Lang)
	}

	switch content.Type {
	case files.TypeImplicitDir:
		return ""
	case files.TypeSymlink:
		return fmt.Sprintf("%s%%attr(-, %s, %s) %q", doc, owner, group, dest)
	case files.TypeDir:
		return fmt.Sprintf("%%dir %s", attrPath(content.Mode(), owner, group, dest))
	case files.TypeConfig:
		return langPrefix + "%config " + doc + attrPath(content.FileInfo.Mode, owner, group, dest)
	case files.TypeConfigNoReplace:
		return langPrefix + "%config(noreplace) " + doc + attrPath(content.FileInfo.Mode, owner, group, dest)
	case files.TypeConfigMissingOK:
		return langPrefix + "%config(missingok) " + doc + attrPath(content.FileInfo.Mode, owner, group, dest)
	case files.TypeRPMDoc:
		return langPrefix + "%doc " + attrPath(content.FileInfo.Mode, owner, group, dest)
	case files.TypeRPMLicence, files.TypeRPMLicense:
		return langPrefix + "%license " + doc + attrPath(content.FileInfo.Mode, owner, group, dest)
	case files.TypeRPMReadme:
		return langPrefix + "%doc " + attrPath(content.FileInfo.Mode, owner, group, dest)
	case files.TypeRPMGhost:
		mode := content.FileInfo.Mode
		if mode == 0 {
			mode = 0o644
		}
		return "%ghost " + doc + attrPath(mode, owner, group, dest)
	default:
		return langPrefix + doc + attrPath(content.FileInfo.Mode, owner, group, dest)
	}
}

// attrPath renders an "%attr(mode, owner, group) path" entry for the %files
// section, normalizing the mode to its octal RPM representation.
func attrPath(mode fs.FileMode, owner, group, dest string) string {
	return fmt.Sprintf("%%attr(%04o, %s, %s) %q", normalizeFileMode(mode), owner, group, dest)
}

// escapeSpecText doubles every percent sign so that rpmbuild treats the value
// as literal text when the generated SRPM is rebuilt, instead of expanding
// %macro / %(command) / %% sequences. Apply it to user-controlled content only,
// never to the spec directives nfpm itself emits.
func escapeSpecText(s string) string {
	return strings.ReplaceAll(s, "%", "%%")
}
