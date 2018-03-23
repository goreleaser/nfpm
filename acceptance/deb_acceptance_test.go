package acceptance

import "testing"

func TestSimpleDeb(t *testing.T) {
	t.Run("amd64", func(t *testing.T) {
		accept(t, "simple_deb", "simple.yaml", "deb", "deb.dockerfile")
	})
	t.Run("i386", func(t *testing.T) {
		accept(t, "simple_deb_386", "simple.386.yaml", "deb", "deb.386.dockerfile")
	})
}

func TestComplexDeb(t *testing.T) {
	t.Run("amd64", func(t *testing.T) {
		accept(t, "complex_deb", "complex.yaml", "deb", "deb.dockerfile")
	})
	t.Run("i386", func(t *testing.T) {
		accept(t, "complex_deb_386", "complex.386.yaml", "deb", "deb.386.dockerfile")
	})
}
