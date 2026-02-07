/**
 * @Author: zjj
 * @Date: 2025/3/7
 * @Desc:
**/

package activity

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
	"sync"
)

var (
	onceV2      sync.Once
	singletonV2 *actMgrV2
)

type actMgrV2 struct {
	srvActMgr map[uint32]*SrvActList
}

func (v *actMgrV2) addSrvActInfo(req *pb3.FSyncSrvTypeActStatusInfo) {
	delete(v.srvActMgr, req.SrvType)
	v.srvActMgr[req.SrvType] = &SrvActList{
		Infos:    req.Infos,
		EndInfos: req.EndInfos,
	}
}

func (v *actMgrV2) packPb() *pb3.S2C_31_0 {
	var resp pb3.S2C_31_0
	for _, actList := range v.srvActMgr {
		resp.Infos = append(resp.Infos, actList.Infos...)
		resp.EndInfos = append(resp.EndInfos, actList.EndInfos...)
	}
	return &resp
}

func (v *actMgrV2) bro() {
	engine.Broadcast(chatdef.CIWorld, 0, 31, 0, v.packPb(), 0)
}

type SrvActList struct {
	Infos    []*pb3.ActStatusInfo // 未开始的 + 正在进行的
	EndInfos []*pb3.ActStatusInfo // 已结束的
}

func ActMgrV2() *actMgrV2 {
	onceV2.Do(func() {
		singletonV2 = &actMgrV2{}
	})
	if singletonV2.srvActMgr == nil {
		singletonV2.srvActMgr = make(map[uint32]*SrvActList)
	}
	return singletonV2
}

func c2sGetActList(player iface.IPlayer, _ *base.Message) error {
	player.SendProto3(31, 0, ActMgrV2().packPb())
	return nil
}

func init() {
	engine.RegisterSysCall(sysfuncid.FSyncSrvTypeActStatusInfo, onFSyncSrvTypeActStatusInfo)
	net.RegisterProto(31, 0, c2sGetActList)
}

func onFSyncSrvTypeActStatusInfo(buf []byte) {
	var req pb3.FSyncSrvTypeActStatusInfo
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("err:%v", err)
		return
	}
	mgrV2 := ActMgrV2()
	mgrV2.addSrvActInfo(&req)
	if !req.Bro {
		return
	}
	mgrV2.bro()
}
