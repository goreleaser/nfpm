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

![nfpm init](https://user-images.githubusercontent.com/245435/36346038-a11210ee-141d-11e8-9838-f95afa10c4f5.png)

The next step is to run `nfpm pkg --target mypkg.deb`.
NFPM will guess which packager to use based on the target file extension.

![nfpm pkg](https://user-images.githubusercontent.com/245435/36346033-66b4ba50-141d-11e8-8f69-2367f9e96702.png)

And that's it!

### Status

* both deb and rpm packaging are working but there are some missing features;
* we need a suite of acceptance tests to make sure everything works.
