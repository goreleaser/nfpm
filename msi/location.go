package msi

import (
	"github.com/goreleaser/nfpm/v2"
	"go.digitalxero.dev/go-msi"
)

// Persist in the explicitly 32-bit registry view, including for x64 packages,
// matching RegLocator's default view. Architecture and UpgradeCode namespace
// the key; HKCU/HKLM separate installation contexts. This private value is an
// owned component key path, so it is removed on uninstall and updated on upgrade.
func persistInstallFolder(b msi.PackageBuilder, info *nfpm.Info, upgradeCode string) {
	key := `Software\nFPM\MSI\` + upgradeCode + `\` + info.Arch
	root := msi.RegistryRootHKLM
	if info.MSI.PerUser {
		root = msi.RegistryRootHKCU
	}
	b.Directory(installFolder).Component("NfpmInstallLocation").
		WithGUID(deriveGUID(identitySeed("install-location", upgradeCode, info.Arch))).
		WithAttributes(msidbComponentAttributesRegistryKeyPath).
		RegistryKey(root, key).Value("InstallFolder", "[INSTALLFOLDER]").AsKeyPath()
	b.Directory(installFolder).Component("NfpmInstallLocation").AssociateToFeature(mainFeature)
	b.Search("NFPM_SAVED_INSTALLFOLDER").InRegistry(root, key, "InstallFolder").AsRawValue()
	restore := b.CustomAction("NfpmRestoreInstallFolder").
		SetProperty(installFolder, "[NFPM_SAVED_INSTALLFOLDER]")
	for _, sequence := range []msi.SequenceTable{msi.InstallUISequence, msi.InstallExecuteSequence} {
		restore.ScheduleBefore(sequence, "CostInitialize", "NOT INSTALLFOLDER AND NFPM_SAVED_INSTALLFOLDER")
	}
}
