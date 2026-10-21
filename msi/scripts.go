package msi

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/goreleaser/nfpm/v2"
	"go.digitalxero.dev/go-msi"
)

// maxScriptSize bounds the size of an embedded maintainer script.
//
// The script travels base64-encoded inside a PowerShell bootstrap that is
// itself UTF-16LE + base64 encoded into the custom action command line, so
// every script byte costs 4/3 * 2 * 4/3 = 3.56 command-line characters on top
// of the fixed ~1,200-character bootstrap. Windows caps a command line at
// 32,767 characters; an 8 KiB script lands around 30,300, leaving headroom for
// the bootstrap to grow (TestScriptCommandLineFitsCommandLine guards it).
//
// The CustomAction.Target column is declared 255 characters wide in the MSI
// schema. Windows Installer stores and runs longer values fine (the Windows
// acceptance job proves it), but strict validators such as msival2 report the
// overflow as ICE03 "string overflow"; embedding the script is worth that
// warning because it is the only way the hooks can run before any package
// file exists (preinstall) or after every file is gone (postremove).
const maxScriptSize = 8 * 1024

// scriptExtensions maps a supported script extension to the PowerShell
// expression the bootstrap uses to run the extracted script.
// nolint: gochecknoglobals
var scriptExtensions = map[string]string{
	".ps1": "& powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File $f",
	".bat": "& cmd.exe /c $f",
	".cmd": "& cmd.exe /c $f",
}

// scriptHooks returns the root maintainer scripts mapped to their MSI custom
// action identity and InstallExecuteSequence schedule. "NOT Installed" runs the
// install hooks on fresh install and on the new product during a major upgrade
// (deb preinst/postinst semantics); "NOT UPGRADINGPRODUCTCODE" keeps the old
// product's remove hooks from firing while it is being replaced.
func scriptHooks(info *nfpm.Info) []struct {
	field     string
	path      string
	caID      string
	before    bool
	anchor    string
	condition string
} {
	const removeCondition = `REMOVE="ALL" AND NOT UPGRADINGPRODUCTCODE`
	return []struct {
		field     string
		path      string
		caID      string
		before    bool
		anchor    string
		condition string
	}{
		{"preinstall", info.Scripts.PreInstall, "NfpmPreInstall", true, "InstallFiles", "NOT Installed"},
		{"postinstall", info.Scripts.PostInstall, "NfpmPostInstall", false, "InstallFiles", "NOT Installed"},
		{"preremove", info.Scripts.PreRemove, "NfpmPreRemove", true, "RemoveFiles", removeCondition},
		{"postremove", info.Scripts.PostRemove, "NfpmPostRemove", false, "RemoveFiles", removeCondition},
	}
}

// addScripts turns the root maintainer scripts into deferred, elevated custom
// actions. Failures roll back the installer transaction, including on
// uninstall, matching dpkg/rpm strictness.
func addScripts(b msi.PackageBuilder, info *nfpm.Info) error {
	for _, h := range scriptHooks(info) {
		if h.path == "" {
			continue
		}
		cmd, err := scriptCommandLine(h.caID, h.path, is64bit(info.Arch))
		if err != nil {
			return fmt.Errorf("package scripts.%s: %w", h.field, err)
		}
		ca := b.CustomAction(h.caID).
			EXEFromDirectory("TARGETDIR", cmd).
			Deferred().NoImpersonate().HideTarget()
		if h.before {
			ca.ScheduleBefore(msi.InstallExecuteSequence, h.anchor, h.condition)
		} else {
			ca.ScheduleAfter(msi.InstallExecuteSequence, h.anchor, h.condition)
		}
	}
	return nil
}

// scriptCommandLine embeds the script file into a self-deleting PowerShell
// bootstrap and returns the full custom action command line. The script bytes
// travel base64 inside the bootstrap because the payload must run at points
// where no package file is on disk yet (preinstall) or anymore (postremove).
//
// The custom action runs as SYSTEM, whose temp directory (C:\Windows\Temp)
// lets any local user pre-create files, so the extracted script must not have
// a predictable name and must never be written over an existing file: the
// name gets a CSPRNG-backed random component and the file is opened with
// CreateNew, which fails (and rolls the install back) if the path exists.
func scriptCommandLine(caID, path string, is64 bool) (string, error) {
	script, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	run, ok := scriptExtensions[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return "", fmt.Errorf("%q: msi scripts must be .ps1, .bat, or .cmd", path)
	}

	bootstrap := fmt.Sprintf(
		"$f = Join-Path $env:TEMP ('nfpm_%s_' + [IO.Path]::GetRandomFileName() + '%s')\n"+
			"$s = [IO.File]::Open($f, 'CreateNew')\n"+
			"try { $b = [Convert]::FromBase64String('%s'); $s.Write($b, 0, $b.Length) } finally { $s.Dispose() }\n"+
			"try { %s; exit $LASTEXITCODE }\n"+
			"finally { Remove-Item -Force $f -ErrorAction SilentlyContinue }",
		caID, strings.ToLower(filepath.Ext(path)),
		base64.StdEncoding.EncodeToString(script), run,
	)

	// -EncodedCommand takes base64 of the UTF-16LE bootstrap. [System64Folder]
	// is a Formatted property; deferred action targets are formatted at script
	// generation time, when properties are still available.
	folder := "[System64Folder]"
	if !is64 {
		folder = "[SystemFolder]"
	}
	return fmt.Sprintf(
		`"%sWindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand %s`,
		folder, base64.StdEncoding.EncodeToString(utf16le(bootstrap)),
	), nil
}

// validateScripts checks the root maintainer scripts against MSI constraints:
// supported interpreter extensions and the embeddable size limit.
func validateScripts(info *nfpm.Info) error {
	for _, h := range scriptHooks(info) {
		if h.path == "" {
			continue
		}
		if _, ok := scriptExtensions[strings.ToLower(filepath.Ext(h.path))]; !ok {
			return fmt.Errorf("package scripts.%s %q: msi scripts must be .ps1, .bat, or .cmd", h.field, h.path)
		}
		st, err := os.Stat(h.path)
		if err != nil {
			return fmt.Errorf("package scripts.%s: %w", h.field, err)
		}
		if st.Size() > maxScriptSize {
			return fmt.Errorf(
				"package scripts.%s %q is %d bytes; msi embeds scripts in the install command line and supports at most %d bytes",
				h.field, h.path, st.Size(), maxScriptSize,
			)
		}
	}
	return nil
}

func utf16le(s string) []byte {
	codes := utf16.Encode([]rune(s))
	b := make([]byte, len(codes)*2)
	for i, c := range codes {
		binary.LittleEndian.PutUint16(b[i*2:], c)
	}
	return b
}
