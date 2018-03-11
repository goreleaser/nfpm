package acceptance

import "testing"

func TestSimpleDeb(t *testing.T) {
	accept(t, "simple_deb", "deb")
}

func TestComplexDeb(t *testing.T) {
	accept(t, "complex_deb", "deb")
}
