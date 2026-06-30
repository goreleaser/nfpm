package rpm

import (
	"github.com/goreleaser/nfpm/v2"
	"go.digitalxero.dev/rpm"
)

// rpmSenseScriptPost marks a dependency as Requires(post).
// https://github.com/rpm-software-management/rpm/blob/master/include/rpm/rpmds.h
const rpmSenseScriptPost = 1 << 10

// applyRelations maps the nfpm dependency lists onto the package builder.
func applyRelations(b rpm.PackageBuilder, info *nfpm.Info) error {
	addAll := func(set rpm.RelationSetBuilder, items []string) {
		for _, item := range items {
			set.AddParsed(item)
		}
		set.Done()
	}

	addAll(b.Provides(), info.Provides)
	addAll(b.Requires(), info.Depends)
	addAll(b.Recommends(), info.Recommends)
	addAll(b.Obsoletes(), info.Replaces)
	addAll(b.Suggests(), info.Suggests)
	addAll(b.Conflicts(), info.Conflicts)

	// Requires(post) dependencies carry the post-install scriptlet sense.
	for _, item := range info.RPM.Requires.Post {
		rel, err := rpm.ParseRelation(item)
		if err != nil {
			return err
		}
		b.Requires().
			With(rel.Name(), rel.Version(), rel.Sense()|rpm.SenseScriptPost).
			Done()
	}

	return nil
}
