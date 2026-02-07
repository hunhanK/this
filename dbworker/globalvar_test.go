/**
 * @Author: zjj
 * @Date: 2025/4/7
 * @Desc:
**/

package dbworker

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base/excel"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"testing"
)

// 比对运营活动
func TestLoadGlobalVarCacheFromFile(t *testing.T) {
	logger.InitLogger()
	globalVar := LoadGlobalVarCacheFromFile("1743765785")
	if globalVar == nil {
		t.Fatal("globalVar is nil")
	}

	var actor1 uint64 = 9897097824268
	var actor2 uint64 = 9897097824267

	yy1 := globalVar.GlobalPlayerYY[actor1]

	var yyIds pie.Uint32s
	for yyId := range yy1.Info {
		yyIds = yyIds.Append(yyId)
	}
	yy2 := globalVar.GlobalPlayerYY[actor2]
	for yyId := range yy2.Info {
		yyIds = yyIds.Append(yyId)
	}
	yyIds = yyIds.Unique().Sort()

	exporter := excel.NewExporter("运营活动对比.xlsx")
	sheet := exporter.NewSheet("比对")

	sheet.WriterTitle("活动ID", "9897097824268", "confIdx", "模板ID", "开始时间", "结束时间", "9897097824267", "confIdx", "模板ID", "开始时间", "结束时间")
	for _, yyId := range yyIds {
		yy1 := globalVar.GlobalPlayerYY[actor1].Info[yyId]
		if yy1 == nil {
			yy1 = &pb3.YYStatus{}
		}
		yy2 := globalVar.GlobalPlayerYY[actor2].Info[yyId]
		if yy2 == nil {
			yy2 = &pb3.YYStatus{}
		}
		sheet.WriterData(
			yyId,
			"", yy1.ConfIdx, yy1.Class,
			utils.Ternary(yy1.OTime == 0, "", time_util.TimeToStr(yy1.OTime)), utils.Ternary(yy1.ETime == 0, "", time_util.TimeToStr(yy1.ETime)),
			"", yy2.ConfIdx, yy2.Class,
			utils.Ternary(yy2.OTime == 0, "", time_util.TimeToStr(yy2.OTime)), utils.Ternary(yy2.ETime == 0, "", time_util.TimeToStr(yy2.ETime)),
		)
	}

	err := exporter.Export()
	if err != nil {
		t.Errorf("err:%v", err)
		return
	}
}
