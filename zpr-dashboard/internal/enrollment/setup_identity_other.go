//go:build linux || windows

package enrollment

func keychainAvailable() bool { return false }

func loadSetupIdentity(config SetupConfig) (*SoftwareIdentity, error) {
	return LoadSoftwareIdentity(config.StateDirectory)
}

func createSetupIdentity(config SetupConfig, metadata LocalEnrollment) (*SoftwareIdentity, error) {
	return CreateSoftwareIdentity(config.StateDirectory, metadata)
}
