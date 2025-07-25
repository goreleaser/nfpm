# nfpm package

Creates a package based on the given config file and flags

```
nfpm package [flags]
```

## Options

```
  -f, --config string     config file to be used (default "nfpm.yaml")
  -h, --help              help for package
  -p, --packager string   which packager implementation to use [apk|archlinux|deb|ipk|rpm|srpm]
  -t, --target string     where to save the generated package (filename, folder or empty for current folder)
```

## See also

* [nfpm](/cmd/nfpm/)	 - Packages apps on RPM, Deb, APK, Arch Linux, and ipk formats based on a YAML configuration file

