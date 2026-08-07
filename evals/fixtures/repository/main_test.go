package fixture

import "testing"

func TestRun(t *testing.T) {
	if Run() != "ready" {
		t.Fatal("fixture is not ready")
	}
}
