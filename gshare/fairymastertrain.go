/**
 * @Author: zjj
 * @Date: 2024/10/14
 * @Desc:
**/

package gshare

func GetFirstPassFairyMasterLayerMgr() map[uint32]uint64 {
	staticVar := GetStaticVar()
	if staticVar.FirstPassFairyMasterLayer == nil {
		staticVar.FirstPassFairyMasterLayer = make(map[uint32]uint64)
	}
	return staticVar.FirstPassFairyMasterLayer
}
