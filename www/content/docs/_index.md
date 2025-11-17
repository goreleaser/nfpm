---
title: Documentation
cascade:
  type: docs
---

Welcome to the nFPM documentation.

## Why nFPM?

While [fpm](https://github.com/jordansissel/fpm) is great, it depends on Ruby, tar and other software. nFPM is a **simpler**, **zero-dependency**, **minimal-assumptions** alternative.

This is a subtle way of saying it won't have all features, nor all formats that `fpm` has: it is supposed to be simpler. And that's OK! Most of us don't need all those features most of the time.

## Features

- **Zero Dependencies**: No Ruby, no tar, no external dependencies
- **Multiple Formats**: deb, rpm, apk, ipk, and arch linux packages
- **Simple Configuration**: Single YAML file for all package formats
- **Cross Platform**: Build on any platform Go supports
- **Fast**: Written in Go for speed and efficiency

## How does it work?

You create a YAML file with the definition of what you need, run the `nfpm` binary, and it takes care of everything. The same configuration file can be used to create packages in all the supported formats.

## nFPM is not FPM

This is a subtle way of saying it won't have all features, nor all formats that `fpm` has: it is supposed to be simpler.

And that's OK! Most of us don't need all those features most of the time.
