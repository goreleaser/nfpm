package acceptance

import "testing"

func TestSimpleRPM(t *testing.T) {
	accept(t, "simple_rpm", "rpm")
}

func TestComplexRPM(t *testing.T) {
	accept(t, "complex_rpm", "rpm")
}
