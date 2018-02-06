# nfpm

> NFPM is not FPM.

WIP: simple deb/rpm packager written in Go

### Goals

- be simple to use
- provide packaging for the most common linux packaging systems
- be distributed as a single binary
- reproducible results
  - depend on the fewer external things as possible
  - generate packages from yaml files (and/or json/toml?)
- be possible to be used as a lib for other go projects (namely goreleaser itself)

### Status

- deb packaging is working but some features might be missing
- rpm packaging is working but some features might be missing
- we need a suite of acceptance tests to make sure everything works
