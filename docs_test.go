package sitegen_test

import (
	"testing"

	"github.com/Deufel/sitegen"
)

// TestDocumented — the generator holds itself to the discipline it checks.
func TestDocumented(t *testing.T) {
	if err := sitegen.Must(sitegen.Check(".")); err != nil {
		t.Fatal(err)
	}
}
