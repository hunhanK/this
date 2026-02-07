/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/9/26 13:48
 */

package series

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/db"
)

func LoadMailSeries() error {
	rows, err := db.OrmEngine.QueryString("call loadmaxmailidseries()")
	if nil != err {
		logger.LogError("********** loadmaxmailidseries error! server stop!**********")
		return err
	}
	for _, r := range rows {
		MailSeries_ = utils.AtoUint32(r["max_series"])
		logger.LogDebug("mailSeries:%d", MailSeries_)
	}

	return nil
}
