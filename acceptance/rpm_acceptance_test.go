package acceptance

import "testing"

func TestSimpleRPM(t *testing.T) {
	accept(t, "simple_rpm", "simple.yaml", "rpm", "rpm.dockerfile")
}

func TestComplexRPM(t *testing.T) {
	accept(t, "complex_rpm", "complex.yaml", "rpm", "rpm.dockerfile")
}
