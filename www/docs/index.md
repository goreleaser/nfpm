# nFPM

![](/static/banner.svg)

nFPM is a simple, 0-dependencies, `deb`, `rpm` and `apk` packager.

## Why

While [fpm][] is great, for me, it is a bummer that it depends on `ruby`, `tar`
and other softwares.

I wanted something that could be used as a binary and/or as a library and that
was really simple.

So I decided to create nFPM: a **simpler**, **0-dependency**,
**as-little-assumptions-as-possible** alternative to fpm.

## nFPM is not FPM

This is a subtle way of saying it wont have all features, nor all
formats that fpm has: it is supposed to be simpler.

And that's OK!, most of us don't need all those features most of the time.

[fpm]: https://github.com/jordansissel/fpm

## How does it work?

You create a YAML file with the definition of what you need, run the `nfpm`
binary, and it takes care of everything.

The same config file can be used to create both the RPM and Deb packages.
