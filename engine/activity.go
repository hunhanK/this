/**
 * @Author: PengZiMing
 * @Desc:
 * @Date: 2021/11/5 11:48
 */

package engine

var (
	GetActStatusFunc func(actId uint32) uint32
)

func GetActStatus(actId uint32) uint32 {
	return GetActStatusFunc(actId)
}
