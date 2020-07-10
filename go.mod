module github.com/goreleaser/nfpm

go 1.14

require (
	github.com/Masterminds/semver/v3 v3.1.0
	github.com/alecthomas/kingpin v2.2.6+incompatible
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d // indirect
	github.com/blakesmith/ar v0.0.0-20190502131153-809d4375e1fb
	github.com/golangci/golangci-lint v1.28.2
	github.com/google/rpmpack v0.0.0-20200615183209-0c831d19bd44
	github.com/goreleaser/chglog v0.0.0-20191115023842-d969fbb05c3c
	github.com/imdario/mergo v0.3.9
	github.com/mattn/go-zglob v0.0.2
	github.com/op/go-logging v0.0.0-20160315200505-970db520ece7 // indirect
	github.com/pkg/errors v0.9.1
	github.com/sassoftware/go-rpmutils v0.0.0-20190420191620-a8f1baeba37b
	github.com/stretchr/testify v1.6.1
	github.com/ulikunitz/xz v0.5.7 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	gopkg.in/yaml.v2 v2.3.0
)

replace github.com/google/rpmpack => github.com/caarlos0/rpmpack v0.0.0-20200710042532-753d6ca00514
