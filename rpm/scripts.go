package rpm

const scriptCreateUser = `
getent group %{package_user} > /dev/null || groupadd -r %{package_user}
getent passwd %{package_user} > /dev/null || \
    useradd -r -d %{_localstatedir}/lib/%{package_user} -g %{package_user} \
    -s /sbin/nologin %{package_user}
exit 0
`
