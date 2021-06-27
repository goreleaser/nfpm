# nfpm package

Creates a package based on the given the given config file and flags

```
nfpm package [flags]
```

## Options

```
  -f, --config string     Config file to be used (default "nfpm.yaml")
  -h, --help              help for package
  -p, --packager string   Which packager implementation to use [apk|deb|rpm]
  -t, --target string     Where to save the generated package (filename, folder or empty for current folder)
```

## See also

* [nfpm](/cmd/nfpm/)	 - Packages apps on RPM, Deb and APK formats based on a YAML configuration file

