package pxcbackup

// removeStringsFromSlice removes `removal` string from `seq` slice
func removeStringsFromSlice(seq []string, removal string) []string {
	for i := 0; i != len(seq); {
		if seq[i] == removal {
			seq = append(seq[:i], seq[i+1:]...)
		} else {
			i++
		}
	}

	return seq
}
