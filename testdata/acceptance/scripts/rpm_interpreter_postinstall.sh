# bash enables its POSIX mode when it is run as /bin/sh.
if shopt -qo posix; then echo sh; else echo bash; fi > /tmp/postinstall-interpreter
