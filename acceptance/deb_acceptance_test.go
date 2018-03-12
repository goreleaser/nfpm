package acceptance

import "testing"

func TestSimpleDeb(t *testing.T) {
	accept(t, "simple_deb", "simple.yaml", "deb", "deb.dockerfile")
}

func TestComplexDeb(t *testing.T) {
	accept(t, "complex_deb", "complex.yaml", "deb", "deb.dockerfile")
}
