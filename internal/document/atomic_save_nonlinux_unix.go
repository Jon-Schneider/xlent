//go:build !windows && !linux

package document

func preserveDestinationPermissions(string, string, bool) error {
	return nil
}
