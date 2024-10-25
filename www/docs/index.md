# nFPM

![](https://becker.software/nfpm.png)

**nFPM is Not FPM** - a zero dependencies, simple `deb`, `rpm`, `apk`, `ipk`, and
arch linux packager written in Go.

## Why

While [fpm][] is great, for me, it is a bummer that it depends on `ruby`, `tar`
and other software.

I wanted something that could be used as a binary and/or as a library and that
was really simple.

So I decided to create nFPM: a **simpler**, **0-dependency**,
**as-little-assumptions-as-possible** alternative to fpm.

## nFPM is not FPM

This is a subtle way of saying it won't have all features, nor all
formats that `fpm` has: it is supposed to be simpler.

And that's OK! Most of us don't need all those features most of the time.

[fpm]: https://github.com/jordansissel/fpm

## How does it work?

You create a YAML file with the definition of what you need, run the `nfpm`
binary, and it takes care of everything.
The same configuration file can be used to create packages in all the supported
formats.
