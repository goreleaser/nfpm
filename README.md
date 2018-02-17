# nfpm

> NFPM is Not FPM - a simple deb and rpm packager written in Go

### Goals

* be simple to use
* provide packaging for the most common linux packaging systems (at very least deb and rpm)
* be distributed as a single binary
* reproducible results
  * depend on the fewer external things as possible (namely `rpmbuild`)
  * generate packages based on yaml files (maybe also json and toml?)
* be possible to use it as a lib in other go projects (namely goreleaser itself)

### Usage

The first steps are to run `nfpm init` to initialize a config file and edit
the generated file according to your needs:

![nfpm init](https://user-images.githubusercontent.com/245435/36346101-f81cdcec-141e-11e8-8afc-a5eb93b7d510.png)

The next step is to run `nfpm pkg --target mypkg.deb`.
NFPM will guess which packager to use based on the target file extension.

![nfpm pkg](https://user-images.githubusercontent.com/245435/36346100-eaaf24c0-141e-11e8-8345-100f4d3ed02d.png)

And that's it!

### Status

* both deb and rpm packaging are working but there are some missing features;
* we need a suite of acceptance tests to make sure everything works.
