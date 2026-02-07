package entity

import (
	"jjyz/base"
	"jjyz/base/pb3"
	"testing"
)

func TestGetYYActiveId(t *testing.T) {
	var a = &pb3.YYBaseDriveTest{
		Base: &pb3.YYBase{
			ActiveId: 123,
		},
		F1: "hhh",
	}

	msg := base.NewMessage()
	msg.SetCmd(31<<8 | 12)
	err := msg.PackPb3Msg(a)
	if err != nil {
		panic(err)
	}

	var st pb3.YYBaseDriveTmpl
	err = msg.UnPackPb3Msg(&st)
	if err != nil {
		panic(err)
	}

	t.Logf("%+v", st)
}
