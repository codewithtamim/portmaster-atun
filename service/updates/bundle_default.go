//go:build !android

package updates

func (u *Updater) loadInstalledIndex(path string) (*Index, error) {
	return LoadIndex(path, u.cfg.Platform, u.cfg.Verify)
}

func (u *Updater) verifyInstalled(index *Index) error {
	return index.VerifyArtifacts(u.cfg.Directory)
}

func (u *Updater) skipsArtifact(*Artifact) bool {
	return false
}
