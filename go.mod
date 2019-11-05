module github.com/goreleaser/nfpm

go 1.13

require (
	github.com/Masterminds/semver/v3 v3.0.2
	github.com/alecthomas/kingpin v2.2.6+incompatible
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d // indirect
	github.com/blakesmith/ar v0.0.0-20190502131153-809d4375e1fb
	github.com/google/rpmpack v0.0.0-20191101142923-13d81472ccfe
	github.com/goreleaser/chglog v0.0.0-20191115023842-d969fbb05c3c
	github.com/imdario/mergo v0.3.8
	github.com/mattn/go-zglob v0.0.1
	github.com/op/go-logging v0.0.0-20160315200505-970db520ece7 // indirect
	github.com/pkg/errors v0.8.1
	github.com/sassoftware/go-rpmutils v0.0.0-20190420191620-a8f1baeba37b
	github.com/stretchr/testify v1.4.0
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	gopkg.in/src-d/go-git.v4 v4.13.1
	gopkg.in/yaml.v2 v2.2.5
)

// TODO: remove this when https://github.com/google/rpmpack/pull/34 gets merged in.
replace github.com/google/rpmpack => github.com/djgilcrease/rpmpack v0.0.0-20191106192924-0d61a9631ca8

// TODO: remove this once https://github.com/src-d/go-git/pull/1243 gets merged
replace gopkg.in/src-d/go-git.v4 => github.com/djgilcrease/go-git v0.0.0-20191115023449-e52680bfbcf1
