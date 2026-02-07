/**
 * @Author: PengZiMing
 * @Desc:
 * @Date: 2021/10/12 15:38
 */

package iactorsys

type IBossTip interface {
	CheckBossTip(bossId uint32) bool
}
