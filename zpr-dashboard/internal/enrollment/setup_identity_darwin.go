package enrollment

func loadSetupIdentity(config SetupConfig) (*SoftwareIdentity, error) {
	if config.KeyProtection == "macos-keychain" {
		return loadKeychainIdentity(config.StateDirectory, macKeychain{})
	}
	return LoadSoftwareIdentity(config.StateDirectory)
}

func createSetupIdentity(config SetupConfig, metadata LocalEnrollment) (*SoftwareIdentity, error) {
	if config.KeyProtection == "macos-keychain" {
		return createKeychainIdentity(config.StateDirectory, metadata, macKeychain{})
	}
	return CreateSoftwareIdentity(config.StateDirectory, metadata)
}
