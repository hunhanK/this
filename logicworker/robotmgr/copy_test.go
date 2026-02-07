package robotmgr

import (
	"github.com/995933447/reflectutil"
	"testing"
)

func TestCopyField(t *testing.T) {
	type A struct {
	}
	type B struct {
		A *A
		T string
	}

	var (
		B2 B
		B1 B
	)
	if err := reflectutil.CopySameFields(&B2, &B1); err != nil {
		t.Log(err.Error())
	}
}
