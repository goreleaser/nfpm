package deb

const scriptSystemdPostinst = `
if [ "$1" = "configure" ] || [ "$1" = "abort-upgrade" ] || [ "$1" = "abort-deconfigure" ] || [ "$1" = "abort-remove" ] ; then
	if deb-systemd-helper debian-installed #UNITFILE#; then
		# This will only remove masks created by d-s-h on package removal.
		deb-systemd-helper unmask #UNITFILE# >/dev/null || true

		if deb-systemd-helper --quiet was-enabled #UNITFILE#; then
			# Create new symlinks, if any.
			deb-systemd-helper enable #UNITFILE# >/dev/null || true
		fi
	fi

	# Update the statefile to add new symlinks (if any), which need to be cleaned
	# up on purge. Also remove old symlinks.
	deb-systemd-helper update-state #UNITFILE# >/dev/null || true
fi

if [ "$1" = "configure" ] || [ "$1" = "abort-upgrade" ] || [ "$1" = "abort-deconfigure" ] || [ "$1" = "abort-remove" ] ; then
	if [ -d /run/systemd/system ]; then
		systemctl --system daemon-reload >/dev/null || true
		if [ -n "$2" ]; then
			deb-systemd-invoke try-restart #UNITFILE# >/dev/null || true
		fi
	fi
fi
`

const scriptSystemdPrerm = `
if [ -d /run/systemd/system ]; then
	deb-systemd-invoke stop #UNITFILE# >/dev/null || true
fi
`

const scriptSystemdPostrm = `
if [ "$1" = "remove" ]; then
	if [ -x "/usr/bin/deb-systemd-helper" ]; then
		deb-systemd-helper mask #UNITFILE# >/dev/null || true
	fi
fi

if [ "$1" = "purge" ]; then
	if [ -x "/usr/bin/deb-systemd-helper" ]; then
		deb-systemd-helper purge #UNITFILE# >/dev/null || true
		deb-systemd-helper unmask #UNITFILE# >/dev/null || true
	fi
fi
if [ -d /run/systemd/system ]; then
	systemctl --system daemon-reload >/dev/null || true
fi
`

const scriptCreateUser = `
getent group %{package_user} > /dev/null || groupadd -r %{package_user}
getent passwd %{package_user} > /dev/null || \
    useradd -r -d /var/lib/%{package_user} -g %{package_user} \
    -s /sbin/nologin %{package_user}
exit 0
`
