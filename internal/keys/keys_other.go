//go:build !darwin

package keys

func system() vault { return nowhere{} }
