package rpm

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/goreleaser/chglog"
	"github.com/goreleaser/nfpm/v2"
	"go.digitalxero.dev/rpm"
)

const (
	// https://github.com/rpm-software-management/rpm/blob/master/lib/rpmtag.h#L152
	tagChangelogTime = 1080
	// https://github.com/rpm-software-management/rpm/blob/master/lib/rpmtag.h#L153
	tagChangelogName = 1081
	// https://github.com/rpm-software-management/rpm/blob/master/lib/rpmtag.h#L154
	tagChangelogText = 1082

	changelogNotesTemplate = `
{{- range .Changes }}{{$note := splitList "\n" .Note}}
- {{ first $note }}
{{- range $i,$n := (rest $note) }}{{- if ne (trim $n) ""}}
{{$n}}{{end}}
{{- end}}{{- end}}`
)

// changelogEntry is a single, formatted changelog entry ready for either the
// binary header tags or the source-package spec %changelog section.
type changelogEntry struct {
	when  time.Time
	title string // "Packager - Semver"
	notes string // rendered note lines
}

// changelogEntries parses and formats the configured changelog. It returns nil
// when no changelog is configured or it has no entries (emitting empty changelog
// tags would invalidate the package).
func changelogEntries(info *nfpm.Info) ([]changelogEntry, error) {
	if info.Changelog == "" {
		return nil, nil
	}

	changelog, err := info.GetChangeLog()
	if err != nil {
		return nil, fmt.Errorf("reading changelog: %w", err)
	}
	if len(changelog.Entries) == 0 {
		return nil, nil
	}

	tpl, err := chglog.LoadTemplateData(changelogNotesTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing RPM changelog template: %w", err)
	}

	entries := make([]changelogEntry, len(changelog.Entries))
	for idx, entry := range changelog.Entries {
		var formattedNotes bytes.Buffer
		if err := tpl.Execute(&formattedNotes, entry); err != nil {
			return nil, fmt.Errorf("formatting changelog notes: %w", err)
		}
		entries[idx] = changelogEntry{
			when:  entry.Date,
			title: fmt.Sprintf("%s - %s", entry.Packager, entry.Semver),
			notes: strings.TrimSpace(formattedNotes.String()),
		}
	}
	return entries, nil
}

// applyChangelog attaches the configured changelog to the binary package builder.
func applyChangelog(b rpm.PackageBuilder, info *nfpm.Info) error {
	entries, err := changelogEntries(info)
	if err != nil {
		return err
	}
	for _, e := range entries {
		b.Changelog().Add(e.when, e.title, e.notes)
	}
	return nil
}

// renderSpecChangelog renders the %changelog section for a generated spec file,
// newest entry first per RPM convention. It returns "" when no changelog is set.
func renderSpecChangelog(info *nfpm.Info) (string, error) {
	entries, err := changelogEntries(info)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].when.After(entries[j].when)
	})

	var b strings.Builder
	b.WriteString("\n%changelog\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "* %s %s\n", e.when.Format("Mon Jan 02 2006"), escapeSpecText(e.title))
		b.WriteString(escapeSpecText(e.notes))
		b.WriteString("\n\n")
	}
	return b.String(), nil
}
