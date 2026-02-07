/**
 * @Author: zjj
 * @Date: 2024/10/29
 * @Desc:
**/

package merge

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"jjyz/base/neterror"
	"os"
	"testing"
)

func TestPostMergeError(t *testing.T) {
	logger.InitLogger()
	PostMergeError(neterror.ParamsInvalidError("测试错误"))
}

func TestID(t *testing.T) {
	t.Log(2199493019651 >> 40)
	t.Log((2199493019651 ^ 2<<40) >> 24)
}

func TestExcSql(t *testing.T) {
	logger.InitLogger()
	var templateName = "ynjg.sql.tpl"
	tmpl, err := ParseTmpl(templateName)
	if nil != err {
		logger.LogError("err:%v", err)
		return
	}
	fileName := fmt.Sprintf("ynjg.sql")
	out, err := os.Create(fileName)
	if nil != err {
		logger.LogError("err:%v", err)
		return
	}
	defer out.Close()
	var merge = &Instance{
		Pf:     "u3dv1",
		Master: 24,
		Slave:  []int{25},
	}
	err = tmpl.Execute(out, merge)
	if nil != err {
		logger.LogError("err:%v", err)
		return
	}
}
