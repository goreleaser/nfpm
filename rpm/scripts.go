package rpm

const scriptCreateUser = `
getent group %{package_user} > /dev/null || groupadd -r %{package_user}
getent passwd %{package_user} > /dev/null || \
    useradd -r -d /var/lib/%{package_user} -g %{package_user} \
    -s /sbin/nologin %{package_user}
exit 0
`

const scriptSystemdPostinst = `
if [ $1 -eq 1 ] ; then
        # Initial installation
        systemctl preset %{package_unit} >/dev/null 2>&1 || :
fi
`

const scriptSystemdPreun = `
if [ $1 -eq 0 ] ; then
        # Package removal, not upgrade
        systemctl --no-reload disable %{package_unit} > /dev/null 2>&1 || :
        systemctl stop %{package_unit} > /dev/null 2>&1 || :
fi
`

const scriptSystemdPostun = `
systemctl daemon-reload >/dev/null 2>&1 || :
if [ $1 -ge 1 ] ; then
        # Package upgrade, not uninstall
        systemctl try-restart %{package_unit} >/dev/null 2>&1 || :
fi
`
