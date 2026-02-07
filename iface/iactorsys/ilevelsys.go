/**
* @Author: ChenJunJi
* @Desc:
* @Date: 2021/7/14 21:15
 */

package iactorsys

import "jjyz/base/pb3"

type ILevelSys interface {
	SetLevel(level uint32, logId pb3.LogId)
	GetLevel() uint32
}
