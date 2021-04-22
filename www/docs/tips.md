# Tips, Hints, and useful information

## General maintainability of your packages
* Try hard to make all files work on all platforms you support. 
    * Maintaining separate scripts, config, service files, etc for each platform quickly becomes difficult 
* Put as much conditional logic in the pre/post install scripts as possible instead of trying to build it into the nfpm.yaml
* *if* you need to know the packaging system I have found it useful to add a `/etc/path-to-cfg/package.env` that contains `_INSTALLED_FROM=apk|deb|rpm` which can be sourced into the pre/post install/remove scripts
* *if/when* you need to ask questions during the installation process, create an `install.sh` || `setup.sh` script that asks those questions and stores the answers as env vars in `/etc/path-to-cfg/package.env` for use by the pre/post install/remove scripts
  * If you only need to support deb packages you can use the debconf template/config feature, but since rpm does not support this I would try to unify the way you ask questions.

## Pre/Post Install/Remove/Upgrade scripts
* [APK Docs](https://wiki.alpinelinux.org/wiki/Creating_an_Alpine_package#install)
* [RPM Docs](https://docs.fedoraproject.org/en-US/packaging-guidelines/Scriptlets/)
* [DEB Docs](https://www.debian.org/doc/debian-policy/ch-maintainerscripts.html)

### Example Multi platform post-install script
```bash
#!/bin/sh

# Step 1, decide if we should use systemd or init/upstart
use_systemctl="True"
systemd_version=0
if ! command -V systemctl >/dev/null 2>&1; then
  use_systemctl="False"
else
    systemd_version=$(systemctl --version | head -1 | sed 's/systemd //g')
fi

cleanup() {
    # This is where you remove files that were not needed on this platform / system
    if [ "${use_systemctl}" = "False" ]; then
    	rm -f /path/to/<SERVICE NAME>.service
    else
        rm -f /etc/chkconfig/<SERVICE NAME>
        rm -f /etc/init.d/<SERVICE NAME>
    fi
}

cleanInstall() {
    printf "\033[32m Post Install of an clean install\033[0m\n"
    # Step 3 (clean install), enable the service in the proper way for this platform
    if [ "${use_systemctl}" = "False" ]; then
        if command -V chkconfig >/dev/null 2>&1; then
          chkconfig --add <SERVICE NAME>
        fi
        
        service <SERVICE NAME> restart ||:
    else
    	# rhel/centos7 cannot use ExecStartPre=+ to specify the pre start should be run as root
    	# even if you want your service to run as non root.
        if [ "${systemd_version}" -lt 231 ]; then
	        printf "\033[31m systemd version %s is less then 231, fixing the service file \033[0m\n" "${systemd_version}"
	        sed -i "s/=+/=/g" /path/to/<SERVICE NAME>.service
	    fi
        printf "\033[32m Reload the service unit from disk\033[0m\n"
        systemctl daemon-reload ||:
        printf "\033[32m Unmask the service\033[0m\n"
        systemctl unmask <SERVICE NAME> ||:
        printf "\033[32m Set the preset flag for the service unit\033[0m\n"
        systemctl preset <SERVICE NAME> ||:
        printf "\033[32m Set the enabled flag for the service unit\033[0m\n"
        systemctl enable <SERVICE NAME> ||:
        systemctl restart <SERVICE NAME> ||:
    fi
}

upgrade() {
    printf "\033[32m Post Install of an upgrade\033[0m\n"
    # Step 3(upgrade), do what you need
    ...
}

# Step 2, check if this is a clean install or an upgrade
action="$1"
if  [ "$1" = "configure" ] && [ -z "$2" ]; then
  # Alpine linux does not pass args, and deb passes $1=configure
  action="install"
elif [ "$1" = "configure" ] && [ -n "$2" ]; then
    # deb passes $1=configure $2=<current version>
    action="upgrade"
fi

case "$action" in
  "1" | "install")
    cleanInstall
    ;;
  "2" | "upgrade")
    printf "\033[32m Post Install of an upgrade\033[0m\n"
    upgrade
    ;;
  *)
    # $1 == version being installed  
    printf "\033[32m Alpine\033[0m"
    cleanInstall
    ;;
esac

# Step 4, clean up unused files, yes you get a warning when you remove the package, but that is ok. 
cleanup
```
### Example Multi platform (RPM & Deb) post-remove script
```bash
#!/bin/sh

cleanInstall() {
    printf "\033[32m Post Install of a clean install\033[0m\n"
}

upgrade() {
    printf "\033[32m Post Install of an upgrade\033[0m\n"
}

action="$1"
if  [ "$1" = "configure" ] && [ -z "$2" ]; then
  # Alpine linux does not pass args, and deb passes $1=configure
  action="install"
elif [ "$1" = "configure" ] && [ -n "$2" ]; then
    # deb passes $1=configure $2=<current version>
    action="upgrade"
fi

case "$action" in
  "1" | "install")
    printf "\033[32m Post Install of a clean install\033[0m\n"
    cleanInstall
    ;;
  "2" | "upgrade")
    printf "\033[32m Post Install of an upgrade\033[0m\n"
    upgrade
    ;;
  *)
    # $1 == version being installed  
    printf "\033[32m Alpine\033[0m"
    cleanInstall
    ;;
esac

```

### Deb & RPM
* On upgrade, the scripts are being executed in the following order:
    1. `pretrans` of new package (only applies to RPM)
    2. `preinstall` of new package
    3. `postinstall` of new package
    4. `preremove` of old package
    5. `postremove` of old package
    6. `posttrans` of new package (only applies to RPM)

## Systemd and upstart/init
### upstart / init
* try to just say no to supporting this, but if you must make sure you have a single script that works on all platforms you need to support.
  * as the `post-install` script above does.

### Systemd
* The docs you find for systemd are generally for the latest and greatest version, and it can be hard to find docs for older versions.
  * In the above `post-install` script you see I am doing a systemd version check to correct the `ExecStartPre=+...` and `ExecStop=+...` lines
* You should always use [Table 2. Automatic directory creation and environment variables](https://www.freedesktop.org/software/systemd/man/systemd.exec.html#id-1.14.4.3.6.2)
  * With the note that only `RuntimeDirectory` is used in systemd < 231
* `/bin/bash -c "$(which ...) ...` is a great way to make your single service file work on all platforms since rhel and debian based systems have standard executables in differing locations and complain about `executable path is not absolute`
  * eg `/bin/bash -c '$(which mkdir) -p /var/log/your-service'` 