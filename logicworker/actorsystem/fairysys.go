package actorsystem

import (
	"encoding/json"
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
	"time"

	"github.com/gzjjyz/srvlib/utils/pie"

	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
)

const (
	TargetFairy  = 0
	TargetPlayer = 1
)

type FairySystem struct {
	Base
	battlePos uint32
	skillInfo map[uint64]map[uint32]int64
	status    map[uint64]*pb3.SyncFairyStatus
}

func (sys *FairySystem) GetData() *pb3.FairyData {
	binary := sys.GetBinaryData()
	fairyData := binary.GetFairyData()
	return fairyData
}

func (sys *FairySystem) GetEquipData(slot uint32) *pb3.FairyEquipData {
	fairyData := sys.GetData()
	if fairyData.PosEquip[slot] == nil {
		fairyData.PosEquip[slot] = &pb3.FairyEquipData{}
	}
	equipData := fairyData.PosEquip[slot]
	if equipData.PosData == nil {
		equipData.PosData = make(map[uint32]uint64)
	}
	return equipData
}

func (sys *FairySystem) OnInit() {
	mainData := sys.GetMainData()
	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}
	sys.LoginClear()
	binary := sys.GetBinaryData()

	if nil == binary.GetFairyData() {
		binary.FairyData = &pb3.FairyData{}
	}

	if nil == binary.FairyData.Collection {
		binary.FairyData.Collection = make(map[uint32]uint32)
	}
	if nil == binary.FairyData.BattleFairy {
		binary.FairyData.BattleFairy = make(map[uint32]uint64)
	}
	if nil == binary.FairyData.Cd {
		binary.FairyData.Cd = make(map[uint32]uint32)
	}
	if nil == binary.FairyData.HistoricalStar {
		binary.FairyData.HistoricalStar = make(map[uint32]uint32)
	}
	if nil == binary.FairyData.CollectionAttr {
		binary.FairyData.CollectionAttr = make(map[uint32]bool)
	}
	if nil == binary.FairyData.PosEquip {
		binary.FairyData.PosEquip = make(map[uint32]*pb3.FairyEquipData)
	}
	if nil == binary.FairyData.BattleSrcGod {
		binary.FairyData.BattleSrcGod = make(map[uint32]uint32)
	}
}

func (sys *FairySystem) OnLogin() {
	data := sys.GetData()
	if len(data.BattleFairy) > 0 {
		for i := custom_id.FairyLingZhen_MainBegin; i <= custom_id.FairyLingZhen_MainEnd; i++ {
			if _, ok := data.BattleFairy[uint32(i)]; ok {
				sys.CallFairyToBattle(uint32(i), true)
				break
			}
		}
	}
	sys.SendLoginInfo()
	sys.checkFairyCollectionAttr()
}

func (sys *FairySystem) OnReconnect() {
	if sys.battlePos > 0 {
		sys.CallFairyToBattle(sys.battlePos, true)
	}
	sys.SendLoginInfo()
}

func (sys *FairySystem) CheckPosCanLoad(pos uint32) bool {
	fairyData := sys.GetData()
	if !utils.SliceContainsUint32(fairyData.GetPos(), pos) {
		return false
	}
	if !itemdef.IsFairyPos(pos) {
		return false
	}
	return true
}

func (sys *FairySystem) CheckTakeOnPosHandle(st *pb3.ItemSt, pos uint32) bool {
	itemConf := jsondata.GetItemConfig(st.GetItemId())
	if nil == itemConf {
		return false
	}
	if !itemdef.IsFairy(itemConf.Type) {
		return false
	}
	fairyData := sys.GetData()
	for _, hdl := range fairyData.BattleFairy {
		if fairy, _ := sys.GetFairy(hdl); nil != fairy {
			if st.Pos == 0 && fairy.ItemId == st.ItemId && fairy.Pos != pos {
				sys.owner.SendTipMsg(tipmsgid.HasSameFairyInSlot)
				return false
			}
		}
	}
	return sys.CheckPosCanLoad(pos)
}

func (sys *FairySystem) OnOpen() {
	fairyData := sys.GetData()
	fairyData.Pos = append(fairyData.Pos, custom_id.FairyMainPos1)
	sys.SendProto3(27, 5, &pb3.S2C_27_5{Pos: custom_id.FairyMainPos1})
}

func (sys *FairySystem) SendLoginInfo() {
	sys.S2CFairySlot()
	sys.S2CFairyInSlot()
	sys.S2CFairyCollection()
}

func (sys *FairySystem) GetFairySkillCd(hdl uint64, skill uint32) int64 {
	var cd int64
	if nil != sys.skillInfo[hdl] {
		cd = sys.skillInfo[hdl][skill]
		cur := time.Now().UnixMilli()
		if cd > 0 && cd < cur {
			cd = 0
		}
	}
	return cd
}

func (sys *FairySystem) GetFairySkill(hdl uint64) map[uint32]*pb3.SkillInfo {
	skillInfo := make(map[uint32]*pb3.SkillInfo)

	fairy, _ := sys.GetFairy(hdl)
	if nil == fairy {
		return skillInfo
	}

	fairyConf := jsondata.GetFairyConf(fairy.GetItemId())

	if nil == fairyConf || (fairyConf.Attack == 0 && len(fairyConf.Skills) == 0 && nil == fairy.Ext.FairySkill) {
		return nil
	}

	for _, skill := range fairyConf.Skills {
		cd := sys.GetFairySkillCd(hdl, skill)

		skillLv := uint32(1)
		if fairy.Union2 > fairyConf.Star {
			skillLv += fairy.Union2 - fairyConf.Star
		}

		skillInfo[skill] = &pb3.SkillInfo{ //绝技
			Id:    skill,
			Level: skillLv,
			Cd:    cd,
		}
	}

	if fairyConf.Attack > 0 {
		skillInfo[fairyConf.Attack] = &pb3.SkillInfo{ //绝技
			Id:    fairyConf.Attack,
			Level: 1,
			Cd:    0,
		}
	}

	if nil != fairy.Ext.FairySkill {
		for _, skill := range fairy.Ext.FairySkill {
			// 区分技能对象是玩家/仙灵
			conf := jsondata.GetFairyBreakSkillConf(fairy.ItemId, fairy.Ext.FairyBreakLv)
			if conf != nil {
				if conf.SkillId == 0 || conf.SkillLv == 0 {
					continue
				}
				if conf.Target == TargetFairy {
					cd := sys.GetFairySkillCd(hdl, skill.Id)
					skillInfo[skill.Id] = &pb3.SkillInfo{ //绝技
						Id:    skill.Id,
						Level: skill.Level,
						Cd:    cd,
					}
				}
			} else if pie.Uint32s(fairyConf.SkillGive).Contains(skill.Id) {
				cd := sys.GetFairySkillCd(hdl, skill.Id)
				skillInfo[skill.Id] = &pb3.SkillInfo{
					Id:    skill.Id,
					Level: skill.Level,
					Cd:    cd,
				}
			}
		}
	}

	return skillInfo
}

func syncSkillCd(actor iface.IPlayer, buf []byte) {
	var skillInfo pb3.SyncFairySkill
	if err := pb3.Unmarshal(buf, &skillInfo); nil != err {
		actor.LogError("sync fairy skill cd error:%v", err)
		return
	}
	sys, ok := actor.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if ok && skillInfo.Handle > 0 {
		sys.skillInfo[skillInfo.Handle] = skillInfo.SkillCds
	}
}

func syncStatus(actor iface.IPlayer, buf []byte) {
	var msg pb3.SyncFairyStatus
	if err := pb3.Unmarshal(buf, &msg); nil != err {
		actor.LogError("sync fairy skill status error:%v", err)
		return
	}
	sys, ok := actor.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if ok && msg.Handle > 0 {
		sys.status[msg.Handle] = &msg
		sys.SendProto3(27, 53, &pb3.S2C_27_53{
			DieTime: msg.DieTime,
			Handle:  msg.Handle,
		})
	}
	if sys.checkDieAll() {
		err := sys.owner.CallActorFunc(actorfuncid.FairyAllEndFuBen, nil)
		if err != nil {
			sys.LogError("err:%v", err)
		}
	}
}

func TryReCallFairyToBattle(player iface.IPlayer, req *pb3.CheckAndCallFairyToBattle) {
	sys := player.GetSysObj(sysdef.SiFairy).(*FairySystem)
	data := sys.GetData()
	var pos uint32
	for i := custom_id.FairyLingZhen_MainBegin; i <= custom_id.FairyLingZhen_MainEnd; i++ {
		handler, ok := data.BattleFairy[uint32(i)]
		if !ok {
			continue
		}

		if handler == req.Handle {
			pos = uint32(i)
		}

		status, ok := sys.status[handler]
		if !ok {
			return
		}

		if status.Hp != 0 {
			return
		}
	}

	if pos == 0 {
		sys.LogWarn("仙灵(handler:%d)不在主站位", req.Handle)
		return
	}
	sys.LogInfo("仙灵(handler:%d)死亡，尝试召回", req.Handle)

	delete(sys.status, req.Handle)

	sys.CallFairyToBattle(pos, true)
}

func F2GTryReCallFairyToBattle(player iface.IPlayer, buf []byte) {
	var msg pb3.CheckAndCallFairyToBattle
	err := pb3.Unmarshal(buf, &msg)
	if err != nil {
		player.LogError(err.Error())
	}
	TryReCallFairyToBattle(player, &msg)
}

func CallLastBattleFairyOut(player iface.IPlayer, buf []byte) {
	var msg pb3.CallLastBattleFairyOut
	err := pb3.Unmarshal(buf, &msg)
	if err != nil {
		player.LogError(err.Error())
	}

	sys := player.GetSysObj(sysdef.SiFairy).(*FairySystem)
	data := sys.GetData()
	if data.LastBattlePos == 0 {
		return
	}

	handler, ok := data.BattleFairy[data.LastBattlePos]
	if !ok {
		return
	}

	TryReCallFairyToBattle(player, &pb3.CheckAndCallFairyToBattle{
		Handle: handler,
	})
}

func (sys *FairySystem) LoginClear() {
	sys.skillInfo = make(map[uint64]map[uint32]int64)
	sys.status = make(map[uint64]*pb3.SyncFairyStatus)
}

func (sys *FairySystem) CallFairyToBattle(pos uint32, isCheck bool) {
	fairyData := sys.GetData()
	hdl := fairyData.BattleFairy[pos]
	fairy, _ := sys.GetFairy(hdl)
	changeCd := jsondata.GlobalUint("FairyChangeCD")
	if fairyData.Cd[pos] >= time_util.NowSec() && isCheck { //不满足召唤条件
		return
	}
	reBornCd := jsondata.GlobalUint("FairyRebornCD")
	var hp int64
	if nil != sys.status[hdl] {
		if sys.status[hdl].DieTime > 0 && (reBornCd+sys.status[hdl].DieTime > time_util.NowSec()) {
			sys.SendProto3(27, 53, &pb3.S2C_27_53{
				DieTime: sys.status[hdl].DieTime,
				Handle:  sys.status[hdl].Handle,
			})
			sys.status[hdl].Hp = 0
			return
		}
		hp = sys.status[hdl].Hp
	}
	if fairy != nil {
		// 技能作用是玩家
		conf := jsondata.GetFairyBreakSkillConf(fairy.ItemId, fairy.Ext.FairyBreakLv)
		if conf != nil && conf.Target == TargetPlayer {
			sys.owner.LearnSkill(conf.SkillId, conf.SkillLv, true)
		}

		skills := sys.GetFairySkill(hdl)

		// 获取源神的技能
		if id := sys.GetSrcGodByBattlePos(pos); id > 0 {
			if srcGodSys, ok := sys.owner.GetSysObj(sysdef.SiSourceGod).(*SourceGodSys); ok && srcGodSys.IsOpen() {
				skills = srcGodSys.GetSourceGodSkills(id)
			}
		}

		msg := &pb3.BattleFairy{
			Id:         fairy.GetHandle(),
			Pos:        fairy.GetPos(),
			ItemId:     fairy.GetItemId(),
			Skills:     skills,
			Level:      fairy.GetUnion1(),
			Endowments: sys.getEndowments(fairy),
			Hp:         hp,
			Magic:      sys.GetBinaryData().GetFairyMagicData(),
			BreakLv:    sys.getBreakLv(fairy),
			Star:       sys.getStar(fairy),
			SysAttr: map[uint32]*pb3.SysAttr{
				attrdef.FairyAttrs: sys.calcFairyAttrs(fairy),
			},
			SourceGodId: fairyData.BattleSrcGod[pos],
		}
		err := sys.owner.CallActorFunc(actorfuncid.FairyToBattle, msg)
		if err != nil {
			sys.LogError("actor func: %d, err: %v", actorfuncid.FairyToBattle, err)
			return
		}
		sys.battlePos = pos
		fairyData.LastBattlePos = pos
		fairyData.Cd[pos] = time_util.NowSec() + changeCd
		sys.SendProto3(27, 52, &pb3.S2C_27_52{
			Pos: pos,
			Cd:  fairyData.Cd[pos],
		})
	}
}

func (sys *FairySystem) CallFairyToBag(pos uint32) {
	fairyData := sys.GetData()
	hdl := fairyData.BattleFairy[pos]
	fairy, _ := sys.GetFairy(hdl)
	if fairy != nil {
		conf := jsondata.GetFairyBreakSkillConf(fairy.ItemId, fairy.Ext.FairyBreakLv)
		if conf != nil && conf.Target == TargetPlayer {
			sys.owner.ForgetSkill(conf.SkillId, true, true, true)
		}
	}
}

func (sys *FairySystem) c2sFairyToBattle(msg *base.Message) error {
	var req pb3.C2S_27_50
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	pos := req.GetPos()
	if !itemdef.IsFairyMainPos(pos) {
		return neterror.ParamsInvalidError("fairy main pos is not this pos(%d)", pos)
	}
	sys.CallFairyToBattle(pos, true)
	return nil
}

func (sys *FairySystem) c2sSendLastBattle(msg *base.Message) error {
	var req pb3.C2S_27_65
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	sys.SendProto3(27, 63, &pb3.S2C_27_63{
		Pos: sys.GetData().LastBattlePos,
	})
	return nil
}

func (sys *FairySystem) S2CFairyInSlot() {
	fairyData := sys.GetData()
	sys.SendProto3(27, 3, &pb3.S2C_27_3{
		BattleFairy:  fairyData.BattleFairy,
		BattleSrcGod: fairyData.BattleSrcGod,
	})
}

func (sys *FairySystem) S2CFairySlot() {
	if fairyData := sys.GetData(); nil != fairyData {
		sys.SendProto3(27, 4, &pb3.S2C_27_4{
			Pos: fairyData.GetPos(),
		})
	}
}

func (sys *FairySystem) S2CFairyCollection() {
	if fairyData := sys.GetData(); nil != fairyData {
		sys.SendProto3(27, 10, &pb3.S2C_27_10{
			HistoricalStar: fairyData.GetHistoricalStar(),
			Collection:     fairyData.GetCollection(),
		})
		sys.SendProto3(27, 160, &pb3.S2C_27_160{Ids: fairyData.CollectionAttr})
	}
}

func (sys *FairySystem) S2CFairyEquips() {
	sys.SendProto3(27, 44, &pb3.S2C_27_44{
		Data: sys.GetData().PosEquip,
	})
}

func (sys *FairySystem) c2sUnLockSlot(msg *base.Message) error {
	var req pb3.C2S_27_5
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	lzConf := jsondata.GetFairyLingZhenConf(req.GetPos())
	if nil == lzConf {
		return neterror.ParamsInvalidError("fairy lingzhen conf(%d) nil", req.GetPos())
	}
	if lzConf.Level > sys.owner.GetLevel() {
		sys.owner.SendTipMsg(tipmsgid.TpUnlockNotMeet)
		return nil
	}
	if lzConf.Pass > 0 {
		fairyLandSys, ok := sys.owner.GetSysObj(sysdef.SiFairyLand).(*FairyLandSys)
		if !ok || !fairyLandSys.IsOpen() || uint32(fairyLandSys.getLevel()) < lzConf.Pass {
			sys.owner.SendTipMsg(tipmsgid.TpFbPassCondNotEnough)
			return nil
		}
	}
	fairyData := sys.GetData()
	var lastOk bool
	for _, pos := range fairyData.Pos {
		if req.Pos == pos {
			return nil
		}

		if lzConf.Seat == 1 {
			lastOk = true
			break
		}
		conf := jsondata.GetFairyLingZhenConf(pos)
		if conf.ZhenType == lzConf.ZhenType && lzConf.Seat == conf.Seat+1 {
			lastOk = true
			break
		}
	}
	if !lastOk {
		sys.owner.SendTipMsg(tipmsgid.TpLingZhenLastNeedUnlock)
		return nil
	}
	if !itemdef.IsFairyPos(req.GetPos()) {
		return neterror.ParamsInvalidError("cant unlock exceed fairy pos(%d)", req.GetPos())
	}
	if !sys.owner.ConsumeByConf(lzConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogLingZhenUnLock}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	fairyData.Pos = append(fairyData.Pos, req.GetPos())
	sys.SendProto3(27, 5, &pb3.S2C_27_5{Pos: req.GetPos()})
	return nil
}

func (sys *FairySystem) c2sFairyIntoSlot(msg *base.Message) error {
	var req pb3.C2S_27_6
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if sys.owner.GetFightStatus() {
		sys.owner.SendTipMsg(tipmsgid.TpFightingLimit)
	} else {
		sys.TakeOn(req.GetHandle(), req.GetPos())
	}
	return nil
}

func (sys *FairySystem) GetFairy(hdl uint64) (*pb3.ItemSt, error) {
	if fairyBag, ok := sys.owner.GetSysObj(sysdef.SiFairyBag).(*FairyBagSystem); ok {
		item := fairyBag.FindItemByHandle(hdl)
		if nil != item {
			return item, nil
		}
	}
	return nil, neterror.ParamsInvalidError("fairy(%d) not fount", hdl)
}

func (sys *FairySystem) ClearSlot(pos uint32) bool {
	fairyData := sys.GetData()
	hdl := fairyData.BattleFairy[pos]
	item, err := sys.GetFairy(hdl)
	if nil != err {
		for _, itemSt := range sys.GetMainData().ItemPool.FairyBag {
			if itemSt.Pos == pos {
				itemSt.Pos = 0
				sys.LogError("fairy(%d) item(%s) has pos value(%d) but not in fairy battle", hdl, itemSt.ItemId, pos)
			}
		}
		delete(fairyData.BattleFairy, pos)
		sys.LogError("no fairy(%d) in bag", hdl)
		sys.S2CFairyInSlot()
		return false
	}
	delete(sys.skillInfo, item.GetHandle()) //下阵清cd
	delete(sys.status, item.GetHandle())    //下阵清cd
	delete(fairyData.BattleFairy, pos)
	item.Pos = 0
	return true
}

func (sys *FairySystem) LoadSlot(item *pb3.ItemSt, pos uint32) bool {
	if !itemdef.IsFairyPos(pos) {
		sys.LogError("fairy lingzhen not has pos(%d)", pos)
		return false
	}
	fairyData := sys.GetData()
	fairyData.BattleFairy[pos] = item.Handle
	item.Pos = pos
	return true
}

func (sys *FairySystem) GetFairyByBattlePos(pos uint32) *pb3.ItemSt {
	hdl := sys.GetFairyHdlByBattlePos(pos)
	if hdl == 0 {
		return nil
	}
	fairy, err := sys.GetFairy(hdl)
	if err != nil {
		return nil
	}
	return fairy
}

func (sys *FairySystem) GetFairyHdlByBattlePos(pos uint32) uint64 {
	fairyData := sys.GetData()
	if hdl, ok := fairyData.BattleFairy[pos]; ok {
		return hdl
	}
	return 0
}

func (sys *FairySystem) GetSrcGodByBattlePos(pos uint32) uint32 {
	fairyData := sys.GetData()
	if id, ok := fairyData.BattleSrcGod[pos]; ok {
		return id
	}
	return 0
}

func (sys *FairySystem) GetBattlePosBySrcGod(srcGod uint32) uint32 {
	fairyData := sys.GetData()
	for pos, id := range fairyData.BattleSrcGod {
		if id == srcGod {
			return pos
		}
	}
	return 0
}

func (sys *FairySystem) GetBattleFairy() map[uint32]uint64 {
	data := sys.GetData()
	return data.BattleFairy
}

func (sys *FairySystem) HasFairyInSlot(pos uint32) bool {
	fairyData := sys.GetData()
	if hdl, ok := fairyData.BattleFairy[pos]; ok && hdl > 0 {
		return true
	}
	return false
}

func (sys *FairySystem) IsDeath(pos uint32) bool {
	fairyData := sys.GetData()
	hdl, ok := fairyData.BattleFairy[pos]
	if !ok {
		return false
	}
	status := sys.status
	if status == nil {
		return false
	}
	reBornCd := jsondata.GlobalUint("FairyRebornCD")
	nowSec := time_util.NowSec()
	if status[hdl] == nil {
		return false
	}
	if status[hdl].DieTime == 0 {
		return false
	}
	if reBornCd+status[hdl].DieTime < nowSec {
		return false
	}
	return true
}

func (sys *FairySystem) TakeOff(pos uint32) bool {
	if sys.HasFairyInSlot(pos) {
		if sys.IsDeath(pos) {
			sys.GetOwner().SendTipMsg(tipmsgid.TpFairyIsDeath)
			return false
		}
		if !sys.ClearSlot(pos) {
			return false
		}
	}
	sys.SendProto3(27, 7, &pb3.S2C_27_7{Pos: pos})
	sys.ResetSysAttr(attrdef.FairySeat)
	sys.ResetSysAttr(attrdef.FairyStarProp)
	sys.ResetSysAttr(attrdef.SaFairyEquip)
	sys.CallFairyToBag(pos)
	if sys.battlePos == pos {
		sys.owner.CallActorFunc(actorfuncid.FairyCallBack, nil)
	}

	sys.owner.TriggerQuestEventRange(custom_id.QttTakeOnFairy)

	// 如果有上阵源神，也直接下阵
	if itemdef.IsFairyMainPos(pos) && sys.GetSrcGodByBattlePos(pos) > 0 {
		sys.srcGodTakeOff(pos)
		sys.SendProto3(27, 89, &pb3.S2C_27_89{Pos: pos})
	}
	return true
}

func (sys *FairySystem) TakeOn(hdl uint64, pos uint32) bool {
	item, err := sys.GetFairy(hdl)
	if nil != err {
		sys.LogError(err.Error())
		return false
	}
	if !sys.CheckTakeOnPosHandle(item, pos) {
		return false
	}
	// 自己本身在卡槽
	if item.Pos > 0 {
		if !sys.TakeOff(item.Pos) {
			return false
		}
	}
	// 卡槽有仙灵
	if sys.HasFairyInSlot(pos) {
		if !sys.TakeOff(pos) {
			return false
		}
	}
	if !sys.LoadSlot(item, pos) {
		return false
	}

	// 检查仙灵卡槽上的装备, 不满足条件的装备自动卸载
	sys.checkSyncPosEquip(item, pos)

	sys.ResetSysAttr(attrdef.FairySeat)
	sys.ResetSysAttr(attrdef.FairyStarProp)
	sys.ResetSysAttr(attrdef.SaFairyEquip)
	sys.owner.TriggerQuestEventRange(custom_id.QttTakeOnFairy)

	sys.SendProto3(27, 6, &pb3.S2C_27_6{Handle: hdl, Pos: pos})
	return true
}

func (sys *FairySystem) checkSyncPosEquip(fairy *pb3.ItemSt, slot uint32) {
	equipData := sys.GetEquipData(slot)
	for pos := range equipData.PosData {
		sConf := jsondata.GetFEquipSlotConf(pos)
		if sConf == nil {
			continue
		}
		if fairy.Union1 < sConf.FairyLv || fairy.Union2 < sConf.FairyStar {
			sys.takeOff(slot, pos)
		}
	}
}

func (sys *FairySystem) c2sFairyIntoBag(msg *base.Message) error {
	var req pb3.C2S_27_7
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if sys.owner.GetFightStatus() {
		sys.owner.SendTipMsg(tipmsgid.TpFightingLimit)
	} else {
		sys.TakeOff(req.GetPos())
	}
	return nil
}

func (sys *FairySystem) c2sCollect(msg *base.Message) error {
	var req pb3.C2S_27_11
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	fairyData := sys.GetData()
	fairyConf := jsondata.GetFairyConf(req.GetConfId())
	if nil == fairyConf {
		return neterror.ParamsInvalidError("fairy conf(%d) not found", req.GetConfId())
	}
	historyStar := fairyData.HistoricalStar[req.GetConfId()]
	if historyStar == 0 {
		sys.owner.SendTipMsg(tipmsgid.TpFairyCollectionMax)
		return nil
	}
	if fairyData.Collection[req.GetConfId()] >= historyStar {
		sys.owner.SendTipMsg(tipmsgid.TpFairyCollectionMax)
		return nil
	}
	if fairyData.Collection[req.GetConfId()] == 0 { //激活
		if fairyConf.Star > historyStar {
			return neterror.ParamsInvalidError("fairy(%d) minn star bigger than historyStar when active", req.GetConfId())
		}
		fairyData.Collection[req.GetConfId()] = fairyConf.Star
	} else {
		fairyData.Collection[req.GetConfId()]++
	}
	star := fairyData.Collection[req.GetConfId()]
	logArg, _ := json.Marshal(map[string]interface{}{
		"conf": req.GetConfId(),
		"star": star,
	})
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogFairyCollect, &pb3.LogPlayerCounter{
		StrArgs: string(logArg),
	})
	sys.ResetSysAttr(attrdef.FairyCollectionProperty)
	sys.SendProto3(27, 11, &pb3.S2C_27_11{
		ConfId: req.GetConfId(),
		Star:   star,
	})
	return nil
}

func (sys *FairySystem) c2sFairyBack(msg *base.Message) error {
	var req pb3.C2S_27_12
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	limitNum := jsondata.GlobalUint("FairyBackStarNumLimit")
	if uint32(len(req.GetIds())) > limitNum {
		return neterror.ParamsInvalidError("fairy back num(%d) exceed limit", limitNum)
	}

	handles := req.GetIds()
	//检查仙灵派遣
	if fairyDelegationSys, ok := sys.owner.GetSysObj(sysdef.SiFairyDelegation).(*FairyDelegationSys); ok && fairyDelegationSys.IsOpen() {
		for _, delId := range handles {
			if fairyDelegationSys.IsFairyBeOccupied(delId) {
				return neterror.ParamsInvalidError("fairy in delegation")
			}
		}
	}

	limitStar := jsondata.GlobalUint("FairyBackStarLimit")
	reTalentItemId := jsondata.GlobalUint("FairyReTalentItemId")
	expMoneyType := jsondata.GlobalUint("FairyMoneyType")
	var talentCount int64
	var moneyNum int64
	var hdlL1, hdlL2, hdlL3 []uint64

	handleMap := make(map[uint64]bool)
	for _, handle := range handles {
		fairy, err := sys.GetFairy(handle)
		if nil != err {
			return err
		}
		if handleMap[handle] {
			return neterror.ParamsInvalidError("cant back fairy hdl(%d) repeat", handle)
		}
		handleMap[handle] = true
		if itemdef.IsFairyPos(fairy.Pos) {
			return neterror.ParamsInvalidError("cant back fairy hdl(%d) in pos", handle)
		}

		fairyConf := jsondata.GetFairyConf(fairy.GetItemId())
		if fairyConf == nil {
			return neterror.ParamsInvalidError("cant back fairy itemId(%d) not found", fairy.ItemId)
		}

		if sys.isCultivate(fairy) {
			hdlL1 = append(hdlL1, handle)
		} else {
			if sys.isStarUp(fairy) {
				hdlL2 = append(hdlL2, handle)
			} else if fairy.Union2 <= limitStar {
				hdlL3 = append(hdlL3, handle)
			}
		}
	}

	// 养成仙灵回退返还
	for _, handle := range hdlL1 {
		fairy, _ := sys.GetFairy(handle)
		lv := fairy.Union1
		unit := fairy.Ext.FairyBackNum
		talentCount += unit                                          //洗髓丹
		total := jsondata.GetFairyBackExpMgrConf(fairy.ItemId, lv-1) // 经验货币
		moneyNum += total
	}

	// 只升星仙灵回退返还
	var fairyItems jsondata.StdRewardVec
	for _, handle := range hdlL2 {
		fairy, _ := sys.GetFairy(handle)
		// 本体
		fairyItems = append(fairyItems, &jsondata.StdReward{
			Id:    fairy.ItemId,
			Count: 1,
		})

		// 旧的仙灵starConsume没数据
		if fairy.Ext.StarConsume == nil {
			starConf := jsondata.GetFairyStarConf(fairy.ItemId, fairy.Union2)
			if starConf != nil {
				fairyItems = append(fairyItems, &jsondata.StdReward{
					Id:    fairy.ItemId,
					Count: int64(starConf.StarBackCount),
				})
				for _, v := range starConf.StarBack {
					fairyItems = append(fairyItems, &jsondata.StdReward{
						Id:    v.BackItemId,
						Count: int64(v.BackItemCount),
					})
				}
			}
		} else {
			for itemId, count := range fairy.Ext.StarConsume {
				fairyItems = append(fairyItems, &jsondata.StdReward{
					Id:    itemId,
					Count: int64(count),
				})
			}
		}
	}

	// 初始仙灵且星级<=2仙灵回退返还
	for _, handle := range hdlL3 {
		fairy, _ := sys.GetFairy(handle)
		fairyConf := jsondata.GetFairyConf(fairy.GetItemId())
		moneyNum += fairyConf.SeparateNum
	}

	var rewards []*jsondata.StdReward
	if talentCount > 0 {
		rewards = append(rewards, &jsondata.StdReward{
			Id:    reTalentItemId,
			Count: talentCount,
			Bind:  false,
		})
	}

	// 养成仙灵回退处理
	for _, handle := range hdlL1 {
		fairy, _ := sys.GetFairy(handle)
		fairy.Union1 = 1
		fairy.Ext.FairyBackNum = 0
		fairy.Ext.FairyBreakLv = 1
		fairy.Attrs = nil
		fairy.Attrs2 = nil
		sys.SyncItemChange(fairy, pb3.LogId_LogFairyBack)
		onFairyLvChangeQuest(sys.owner, fairy.GetItemId())
	}

	// 升星仙灵回退处理
	for _, handle := range hdlL2 {
		sys.owner.RemoveFairyByHandle(handle, pb3.LogId_LogFairyBack)
	}

	// 初始仙灵且星级<=2仙灵回退处理
	for _, handle := range hdlL3 {
		sys.owner.RemoveFairyByHandle(handle, pb3.LogId_LogFairyBack)
	}

	// 返还货币
	if moneyNum > 0 {
		sys.owner.AddMoney(expMoneyType, moneyNum, true, pb3.LogId_LogFairyBack)
	}

	// 返还道具
	rewards = append(rewards, fairyItems...)
	rewards = jsondata.MergeStdReward(rewards)
	if len(rewards) > 0 {
		sys.giveFairyBackRewards(rewards)
	}
	sys.SendProto3(27, 12, &pb3.S2C_27_12{Ret: true})
	return nil
}

func (sys *FairySystem) c2sTalentRefresh(msg *base.Message) error {
	var req pb3.C2S_27_13
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	fairy, err := sys.GetFairy(req.GetHandle())
	if nil != err {
		return err
	}
	fairyConf := jsondata.GetFairyConf(fairy.ItemId)
	if nil == fairyConf {
		return neterror.ParamsInvalidError("fairy conf(%d) not found", fairy.ItemId)
	}
	if nil != fairy.Attrs2 { //未确认结果
		sys.owner.SendTipMsg(tipmsgid.TpFairyReTalentNotCheck)
		return nil
	}
	breakLv := sys.getBreakLv(fairy)
	breakConf := jsondata.GetFairyBreakConf(breakLv)
	if nil == breakConf {
		return neterror.ParamsInvalidError("fairy breakConf(%d) conf nil", breakLv)
	}
	var cnt int
	for _, attr := range fairy.Attrs {
		value := sys.GetFairyEndowmentMax(fairy, attr.Type)
		if value > 0 && attr.Value >= value {
			cnt++
		}
	}
	if cnt >= custom_id.FairyTalent_Num {
		return nil
	}
	times := req.GetTimes()
	itemId := jsondata.GlobalUint("FairyReTalentItemId")
	num := breakConf.RefreshNeed * int64(times)
	itemMap := make(map[uint32]int64)
	itemMap[itemId] = num
	consume := jsondata.ConsumeVec{
		{Type: custom_id.ConsumeTypeItem, Id: itemId, Count: uint32(num)},
	}

	if !sys.owner.ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairyReTalent}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	totalNum := sys.getRefineBackNum(fairy)
	totalNum += breakConf.RefreshNeed * int64(req.Times)
	sys.updateRefineBackNum(fairy, totalNum)
	pool := new(random.Pool)
	pool.AddItem(custom_id.FairyReTalent_Add, breakConf.AddRate)
	pool.AddItem(custom_id.FairyReTalent_Reduce, breakConf.ReduceRate)
	pool.AddItem(custom_id.FairyReTalent_Keep, breakConf.KeepRate)
	var newAttr []*pb3.AttrSt
	for talentType := custom_id.FairyTalent_Begin; talentType <= custom_id.FairyTalent_End; talentType++ {
		var srcNum uint32
		for _, attr := range fairy.Attrs {
			if attr.Type == uint32(talentType) {
				srcNum = attr.Value
				break
			}
		}
		for i := uint32(1); i <= times; i++ {
			line := pool.RandomOne()
			if rateType, ok := line.(int); ok {
				switch rateType {
				case custom_id.FairyReTalent_Add:
					if len(breakConf.AddArea) >= 2 {
						add := random.IntervalUU(breakConf.AddArea[0], breakConf.AddArea[1])
						value := sys.GetFairyEndowmentMax(fairy, uint32(talentType))
						if value > 0 && srcNum+add > value {
							srcNum = value
						} else {
							srcNum += add
						}
					}
				case custom_id.FairyReTalent_Reduce:
					if len(breakConf.ReduceArea) >= 2 {
						reduce := random.IntervalUU(breakConf.ReduceArea[0], breakConf.ReduceArea[1])
						if reduce > srcNum {
							srcNum = 0
						} else {
							srcNum -= reduce
						}
					}
				case custom_id.FairyReTalent_Keep:
					//nothing
				}
			}
		}
		newAttr = append(newAttr, &pb3.AttrSt{
			Type:  uint32(talentType),
			Value: srcNum,
		})
	}
	fairy.Attrs2 = newAttr
	sys.SyncItemChange(fairy, pb3.LogId_LogFairyReTalent)
	rsp := &pb3.S2C_27_13{
		Handle: req.GetHandle(),
		Talent: newAttr,
		CosNum: fairy.Ext.FairyBackNum,
	}
	sys.SendProto3(27, 13, rsp)
	logArg, _ := json.Marshal(rsp)
	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogFairyReTalent, &pb3.LogPlayerCounter{
		NumArgs: uint64(fairy.GetItemId()),
		StrArgs: string(logArg),
	})
	return nil
}

func (sys *FairySystem) c2sTalentCheck(msg *base.Message) error {
	var req pb3.C2S_27_14
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	fairy, err := sys.GetFairy(req.GetHandle())
	if nil != err {
		return err
	}
	if nil == fairy.Attrs2 {
		return nil
	}
	if req.GetOp() == custom_id.FairyTalentOpReplace {
		fairy.Attrs = fairy.Attrs2
		fairy.Attrs2 = nil
	} else if req.GetOp() == custom_id.FairyTalentOpGiveUp {
		fairy.Attrs2 = nil
	}
	sys.SyncItemChange(fairy, pb3.LogId_LogFairyReplaceTalent)
	rsp := &pb3.S2C_27_14{Handle: req.GetHandle(), Talent: fairy.Attrs, Op: req.GetOp()}
	sys.SendProto3(27, 14, rsp)

	logArg, _ := json.Marshal(rsp)
	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogFairyReplaceTalent, &pb3.LogPlayerCounter{
		NumArgs: uint64(fairy.GetItemId()),
		StrArgs: string(logArg),
	})
	sys.onEndowmentsChange(fairy)
	return nil
}

func (sys *FairySystem) giveFairyBackRewards(rewards jsondata.StdRewardVec) {
	batchCount := int(jsondata.GetCommonConf("fairyBackCount").U32)

	var batchRewards jsondata.StdRewardVec
	curBatchCount := 0

	for _, reward := range rewards {
		remain := int(reward.Count)
		for remain > 0 {
			space := batchCount - curBatchCount
			take := remain
			if take > space {
				take = space
			}

			batchRewards = append(batchRewards, &jsondata.StdReward{Id: reward.Id, Count: int64(take)})
			curBatchCount += take
			remain -= take

			if curBatchCount == batchCount {
				engine.GiveRewards(sys.owner, batchRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFairyBack})
				batchRewards = nil
				curBatchCount = 0
			}
		}
	}

	if len(batchRewards) > 0 {
		engine.GiveRewards(sys.owner, batchRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFairyBack})
	}
}

func (sys *FairySystem) initStarConsume(fairy *pb3.ItemSt) {
	fairy.Ext.StarConsume = make(map[uint32]uint32)
	starConf := jsondata.GetFairyStarConf(fairy.ItemId, fairy.Union2)
	if starConf == nil {
		return
	}
	if starConf.StarBackCount > 0 {
		fairy.Ext.StarConsume[fairy.ItemId] += starConf.StarBackCount
	}
	for _, v := range starConf.StarBack {
		fairy.Ext.StarConsume[v.BackItemId] += v.BackItemCount
	}
}

func (sys *FairySystem) getBreakLv(fairy *pb3.ItemSt) uint32 {
	if nil != fairy && nil != fairy.Ext {
		return fairy.Ext.FairyBreakLv
	}
	return 0
}

func (sys *FairySystem) getStar(fairy *pb3.ItemSt) uint32 {
	if fairy != nil {
		return fairy.Union2
	}
	return 0
}

func (sys *FairySystem) updateBreakLv(fairy *pb3.ItemSt, lv uint32) bool {
	if nil == fairy {
		return false
	}
	if nil == fairy.Ext {
		return false
	}
	fairy.Ext.FairyBreakLv = lv
	return true
}

func (sys *FairySystem) activeSkill(fairy *pb3.ItemSt) {
	if fairy == nil {
		return
	}
	if fairy.Ext == nil {
		return
	}
	conf := jsondata.GetFairyBreakSkillConf(fairy.ItemId, fairy.Ext.FairyBreakLv)
	if conf == nil {
		return
	}
	if conf.SkillId == 0 || conf.SkillLv == 0 {
		return
	}
	idx := len(fairy.Ext.FairySkill)

	if fairy.Ext.FairySkill == nil {
		fairy.Ext.FairySkill = make(map[uint32]*pb3.Skill)
	}
	fairy.Ext.FairySkill[uint32(idx)] = &pb3.Skill{
		Id:    conf.SkillId,
		Level: conf.SkillLv,
	}
}

func (sys *FairySystem) getRefineBackNum(fairy *pb3.ItemSt) int64 {
	if nil != fairy && nil != fairy.Ext {
		return fairy.Ext.FairyBackNum
	}
	return 0
}

func (sys *FairySystem) updateRefineBackNum(fairy *pb3.ItemSt, times int64) bool {
	if nil == fairy {
		return false
	}
	if nil == fairy.Ext {
		return false
	}
	fairy.Ext.FairyBackNum = times
	return true
}

func (sys *FairySystem) checkG2FSyncFairyAttrs(fairy *pb3.ItemSt) bool {
	return sys.battlePos > 0 && fairy.GetPos() == sys.battlePos
}

func (sys *FairySystem) c2sLevelUp(msg *base.Message) error {
	var req pb3.C2S_27_8
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	fairy, err := sys.GetFairy(req.GetHandle())
	if nil != err {
		return err
	}
	if itemdef.IsFairySupPos(fairy.GetPos()) {
		sys.owner.SendTipMsg(tipmsgid.FairyInSupPos)
		return nil
	}
	lvRefConf := jsondata.GetFairyLvRefConf(fairy.ItemId)
	if lvRefConf == nil || uint32(len(lvRefConf.LevelConf)) <= fairy.Union1 {
		return neterror.ParamsInvalidError("fairy(%d) level max", req.GetHandle())
	}
	lvConf := jsondata.GetFairyLevelConf(fairy.ItemId, fairy.Union1)
	if nil == lvConf {
		return neterror.ParamsInvalidError("fairy level(%d) conf nil", fairy.Union1)
	}
	ntLv := fairy.Union1 + 1
	ntLvConf := jsondata.GetFairyLevelConf(fairy.ItemId, ntLv)
	if nil == ntLvConf {
		return neterror.ParamsInvalidError("fairy level(%d) conf nil", ntLv)
	}
	breakLv := sys.getBreakLv(fairy)
	breakConf := jsondata.GetFairyBreakConf(breakLv)
	if nil == breakConf {
		return neterror.ParamsInvalidError("fairy breakConf(%d) conf nil", breakLv)
	}
	if ntLv > breakConf.LevelMax {
		sys.owner.SendTipMsg(tipmsgid.TpLvUpNeedBreak)
		return nil
	}
	expMoneyType := jsondata.GlobalUint("FairyMoneyType")

	consume := jsondata.ConsumeVec{
		{Type: custom_id.ConsumeTypeMoney, Id: expMoneyType, Count: uint32(lvConf.ItemNum)},
	}

	if !sys.owner.ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairyLvUp}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	fairy.Union1++
	rsp := &pb3.S2C_27_8{Handle: req.GetHandle(), Lv: ntLv}
	sys.SendProto3(27, 8, rsp)
	sys.SyncItemChange(fairy, pb3.LogId_LogFairyLvUp)
	logArg, _ := json.Marshal(rsp)
	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogFairyLvUp, &pb3.LogPlayerCounter{
		NumArgs: uint64(fairy.GetItemId()),
		StrArgs: string(logArg),
	})
	if sys.checkG2FSyncFairyAttrs(fairy) {
		sys.owner.CallActorFunc(actorfuncid.SyncFairyLv, &pb3.CommonSt{
			U64Param: fairy.GetHandle(),
			U32Param: fairy.GetUnion1(),
		})
		sys.owner.CallActorFunc(actorfuncid.G2FCalcFairySysProp, &pb3.SyncFairySysAttrs{
			Id:    attrdef.FairyAttrs,
			Attrs: sys.calcFairyAttrs(fairy),
		})
	}
	onFairyLvChangeQuest(sys.owner, fairy.GetItemId())
	return nil
}

func (sys *FairySystem) UpdateStarMx(itemId, star uint32) {
	fairyData := sys.GetData()
	fairyData.HistoricalStar[itemId] = utils.MaxUInt32(fairyData.HistoricalStar[itemId], star)
	onFairyStarChangeQuest(sys.owner, itemId)
	sys.owner.SendProto3(27, 22, &pb3.S2C_27_22{
		ConfId: itemId,
		Star:   star,
	})
}

func (sys *FairySystem) CanAddBasket(costType int, star, itemId, color, selfItemId uint32, condMap map[int]int64, confList []*jsondata.FairyCommonConsume) (bool, int) {
	length := len(confList)
	// 本体
	if costType == custom_id.FairyCosType_Self && itemId != selfItemId {
		return false, -1
	}
	for i := 0; i < length; i++ { //指定道具
		consume := confList[i]
		if consume.CosType == costType && consume.SpStar == star && consume.Color == color && condMap[i] < consume.ItemNum {
			switch costType {
			case custom_id.FairyCosType_Self:
				return true, i
			case custom_id.FairyCosType_Item:
				if consume.ItemId == itemId {
					return true, i
				}
			case custom_id.FairyCosType_Any:
				return true, i
			}
		}
	}
	return false, -1
}

func (sys *FairySystem) c2sStarUp(msg *base.Message) error {
	var req pb3.C2S_27_15
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if nil == req.Cos {
		return neterror.ParamsInvalidError("fairy up star req nil!")
	}

	//检查仙灵派遣
	if fairyDelegationSys, ok := sys.owner.GetSysObj(sysdef.SiFairyDelegation).(*FairyDelegationSys); ok && fairyDelegationSys.IsOpen() {
		for _, cosSt := range req.Cos {
			for _, delId := range cosSt.CosIds {
				if fairyDelegationSys.IsFairyBeOccupied(delId) {
					return neterror.ParamsInvalidError("fairy in delegation")
				}
			}
		}
	}

	countMap := make(map[uint64]struct{})
	var talentCount, money int64
	var consumes []*jsondata.Consume
	for _, cosSt := range req.Cos {
		fairy, err := sys.GetFairy(cosSt.GetId())
		if nil != err {
			return err
		}
		star := fairy.Union2
		fairyConf := jsondata.GetFairyConf(fairy.ItemId)
		if star+1 > fairyConf.MaxStar {
			return neterror.ParamsInvalidError("fairy(%d) upstart is max", cosSt.GetId())
		}
		colorMin := jsondata.GlobalUint("fairyColorUpMin")
		if fairyConf.Color < colorMin {
			return neterror.ParamsInvalidError("fairy(%d) upstart color(%d) not allow", cosSt.GetId(), fairyConf.Color)
		}
		fairyStarConf := jsondata.GetFairyStarConf(fairy.ItemId, star)
		if nil == fairyStarConf {
			return neterror.ParamsInvalidError("fairy starConf(%d)", star)
		}
		if _, use := countMap[cosSt.GetId()]; use {
			return neterror.ParamsInvalidError("fairy(%d) upstart item reply", cosSt.GetId())
		}
		countMap[cosSt.GetId()] = struct{}{}
		condMap := make(map[int]int64, len(fairyStarConf.CommonConsume))
		if spConf := jsondata.GetFairySpStarUpConf(fairy.GetItemId()); nil != spConf { //特殊仙灵升星
			if len(cosSt.CosIds) > 0 {
				return neterror.ParamsInvalidError("源神不需要消耗仙灵")
			}
			if nil != spConf.Star && nil != spConf.Star[star] {
				consume := jsondata.CopyConsumeVec(spConf.Star[star].Consume)
				consumes = append(consumes, consume...)
			}
		} else { //普通升星
			var checkNum int
			length := len(fairyStarConf.CommonConsume)
			for _, handle := range cosSt.CosIds {
				if _, use := countMap[handle]; use {
					return neterror.ParamsInvalidError("fairy(%d) upstart item reply", cosSt.GetId())
				}
				countMap[handle] = struct{}{}
				cFairy, err := sys.GetFairy(handle)
				if nil != err {
					return err
				}
				cFairyConf := jsondata.GetFairyConf(cFairy.ItemId)
				if nil == cFairyConf {
					return neterror.ParamsInvalidError("fairy conf(%d) not found", cFairy.ItemId)
				}
				//分优先级检验
				for cosType := custom_id.FairyCosType_Begin; cosType <= custom_id.FairyCosType_End; cosType++ {
					if canAdd, idx := sys.CanAddBasket(cosType, cFairy.Union2, cFairy.ItemId, cFairyConf.Color, fairy.ItemId, condMap, fairyStarConf.CommonConsume); canAdd {
						condMap[idx]++
						if condMap[idx] == fairyStarConf.CommonConsume[idx].ItemNum {
							checkNum++
						}
						break
					}
				}
				//返还消耗
				talentCount += cFairy.Ext.FairyBackNum                                   //洗髓丹
				money += jsondata.GetFairyBackExpMgrConf(cFairy.ItemId, cFairy.Union1-1) //经验
			}
			if checkNum != length {
				return neterror.ParamsInvalidError("fairy cos item not meet cond")
			}
		}
	}
	reTalentItemId := jsondata.GlobalUint("FairyReTalentItemId")
	var rewards []*jsondata.StdReward
	rewards = append(rewards, &jsondata.StdReward{
		Id:    reTalentItemId,
		Count: talentCount,
		Bind:  false,
	})

	if ok := engine.CheckRewards(sys.owner, rewards); !ok { //背包空间不足
		sys.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}
	if !sys.owner.ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogFairyStarUp}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	// 记录消耗的仙灵
	cosFairyIdMap := make(map[uint64][]uint32)
	for _, cosSt := range req.Cos {
		fairyIds, ok := cosFairyIdMap[cosSt.GetId()]
		if !ok {
			cosFairyIdMap[cosSt.GetId()] = make([]uint32, 0)
			fairyIds = cosFairyIdMap[cosSt.GetId()]
		}
		for _, handle := range cosSt.CosIds {
			fairy, _ := sys.GetFairy(handle)
			// 消耗的仙灵本身也有记录升星消耗，要叠加进去
			if fairy.Ext != nil && fairy.Ext.StarConsume != nil {
				for consumeId, count := range fairy.Ext.StarConsume {
					for i := uint32(1); i <= count; i++ {
						fairyIds = append(fairyIds, consumeId)
					}
				}
			}
			fairyIds = append(fairyIds, fairy.GetItemId())
		}
		cosFairyIdMap[cosSt.GetId()] = fairyIds
	}

	// 删除消耗的仙灵
	for _, cosSt := range req.Cos {
		for _, handle := range cosSt.CosIds {
			sys.owner.RemoveFairyByHandle(handle, pb3.LogId_LogFairyStarUp)
		}
	}

	for _, cosSt := range req.Cos {
		handle := cosSt.GetId()
		fairy, _ := sys.GetFairy(handle)
		if fairy.Ext.StarConsume == nil {
			sys.initStarConsume(fairy)
		}
		fairyIds := cosFairyIdMap[handle]
		for _, fairyId := range fairyIds {
			fairy.Ext.StarConsume[fairyId] += 1
		}
		fairy.Union2++

		rsp := &pb3.S2C_27_15{Hdl: fairy.GetHandle(), Star: fairy.GetUnion2()}
		sys.SendProto3(27, 15, rsp)
		sys.UpdateStarMx(fairy.GetItemId(), fairy.Union2)
		sys.SyncItemChange(fairy, pb3.LogId_LogFairyStarUp)

		logArg, _ := json.Marshal(rsp)
		logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogFairyStarUp, &pb3.LogPlayerCounter{
			NumArgs: uint64(fairy.GetItemId()),
			StrArgs: string(logArg),
		})

		sys.onEndowmentsChange(fairy)
		fConf := jsondata.GetFairyConf(fairy.ItemId)
		if fairy.Union2 > fConf.Star {
			lv := fairy.Union2 - fConf.Star + 1
			for _, skill := range fConf.Skills {
				sys.onFairyLearnSkill(&pb3.FairyLearnSkillSt{Hdl: handle, SkillId: skill, SkillLevel: lv})
			}
			sys.syncFairyPower(fairy)
		}
		if sys.checkG2FSyncFairyAttrs(fairy) {
			sys.owner.CallActorFunc(actorfuncid.SyncFairyStar, &pb3.CommonSt{
				U64Param: fairy.GetHandle(),
				U32Param: fairy.GetUnion2(),
			})
			sys.owner.CallActorFunc(actorfuncid.G2FCalcFairySysProp, &pb3.SyncFairySysAttrs{
				Id:    attrdef.FairyAttrs,
				Attrs: sys.calcFairyAttrs(fairy),
			})
		}
	}
	sys.checkFairyCollectionAttr()
	expMoneyType := jsondata.GlobalUint("FairyMoneyType")
	engine.GiveRewards(sys.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFairyStarUp})
	if money > 0 {
		sys.owner.AddMoney(expMoneyType, money, true, pb3.LogId_LogFairyStarUp)
	}
	return nil
}

func (sys *FairySystem) c2sBreak(msg *base.Message) error {
	var req pb3.C2S_27_16
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	fairy, err := sys.GetFairy(req.GetHandle())
	if nil != err {
		return err
	}

	breakLv := sys.getBreakLv(fairy)
	conf := jsondata.GetFairyConf(fairy.ItemId)
	if breakLv >= conf.MaxBreakLv {
		return neterror.ParamsInvalidError("breakLv max")
	}

	breakConf := jsondata.GetFairyBreakConf(breakLv)
	fairyData := sys.GetData()
	if uint32(sys.owner.GetExtraAttr(attrdef.Circle)) < breakConf.LevelCondition {
		sys.owner.SendTipMsg(tipmsgid.TpBreakCondNotMeet)
		return nil
	}
	if breakConf.StarLimit > fairy.Union2 {
		sys.owner.SendTipMsg(tipmsgid.TpBreakCondNotMeet)
		return nil
	}
	if breakConf.LevelMax > fairy.Union1 { //自身等级不符合
		if itemdef.IsFairySupPos(fairy.Pos) {
			resonanceLv := uint32(1)
			for i := custom_id.FairyLingZhen_MainBegin; i <= custom_id.FairyLingZhen_MainEnd; i++ {
				if getFairy, _ := sys.GetFairy(fairyData.BattleFairy[uint32(i)]); nil != getFairy {
					if i == custom_id.FairyLingZhen_MainBegin || (getFairy.Union1 < resonanceLv) {
						resonanceLv = getFairy.Union1
					}
				}
			}
			if breakConf.LevelMax > resonanceLv { //共鸣等级不符合
				sys.owner.SendTipMsg(tipmsgid.TpBreakCondNotMeet)
				return nil
			}
		} else { //未共鸣中
			sys.owner.SendTipMsg(tipmsgid.TpBreakCondNotMeet)
			return nil
		}
	}

	ntBreakConf := jsondata.GetFairyBreakConf(breakLv + 1)
	if nil == ntBreakConf {
		sys.owner.SendTipMsg(tipmsgid.TpBreakCondNotMeet)
		return nil
	}
	flag := false
	for _, attr := range fairy.Attrs {
		value := sys.GetFairyEndowmentMax(fairy, attr.Type)
		if value > 0 && attr.Value == value {
			flag = true
			break
		}
	}
	if !flag {
		sys.owner.SendTipMsg(tipmsgid.TpBreakCondNotMeet)
		return nil
	}
	if !sys.owner.ConsumeByConf(breakConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairyBreak}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	sys.updateBreakLv(fairy, breakLv+1)
	sys.activeSkill(fairy)
	rsp := &pb3.S2C_27_16{Handle: fairy.GetHandle(), BreakLv: sys.getBreakLv(fairy)}
	sys.SendProto3(27, 16, rsp)
	sys.SyncItemChange(fairy, pb3.LogId_LogFairyBreak)
	logArg, _ := json.Marshal(rsp)
	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogFairyBreak, &pb3.LogPlayerCounter{
		NumArgs: uint64(fairy.GetItemId()),
		StrArgs: string(logArg),
	})
	if sys.checkG2FSyncFairyAttrs(fairy) {
		sys.owner.CallActorFunc(actorfuncid.SyncFairyBreakLv, &pb3.CommonSt{
			U64Param: fairy.GetHandle(),
			U32Param: sys.getBreakLv(fairy),
		})
		sys.owner.CallActorFunc(actorfuncid.G2FCalcFairySysProp, &pb3.SyncFairySysAttrs{
			Id:    attrdef.FairyAttrs,
			Attrs: sys.calcFairyAttrs(fairy),
		})
	}
	return nil
}

func (sys *FairySystem) GetFairyEndowmentMax(fairy *pb3.ItemSt, endId uint32) uint32 {
	fConf := jsondata.GetFairyConf(fairy.ItemId)
	if nil == fConf {
		return 0
	}

	breakConf := jsondata.GetFairyBreakConf(sys.getBreakLv(fairy))
	if breakConf == nil {
		return 0
	}

	if rConf, ok := fConf.EndowmentsRate[endId]; ok {
		value := float64(breakConf.EndowmentsMax) * (float64(rConf.Value) / custom_id.FairyCoeRate)
		return uint32(value)
	}

	return 0
}

func (sys *FairySystem) GetTotalTalentNoAdd(hdl uint64) map[uint32]*pb3.FairyEndowment {
	fairy, _ := sys.GetFairy(hdl)
	if nil == fairy {
		return nil
	}
	fairyConf := jsondata.GetFairyConf(fairy.GetItemId())
	if nil == fairyConf {
		sys.LogError("fairyConf(%d) not exist", hdl)
		return nil
	}
	grade := fairy.Ext.FairyGrade
	endowments := fairyConf.Endowments
	if nil == endowments {
		sys.LogError("no fairy(%d) grade(%d) conf", fairyConf.ItemID, grade)
		return nil
	}

	ends := make(map[uint32]*pb3.FairyEndowment)

	//基础资质
	energy := endowments[custom_id.FairyTalent_Energy].Value
	power := endowments[custom_id.FairyTalent_Power].Value
	intelligence := endowments[custom_id.FairyTalent_Intelligence].Value
	defense := endowments[custom_id.FairyTalent_Defense].Value
	//洗髓资质
	for _, talent := range fairy.Attrs {
		switch talent.Type {
		case custom_id.FairyTalent_Energy:
			energy += talent.Value
		case custom_id.FairyTalent_Power:
			power += talent.Value
		case custom_id.FairyTalent_Intelligence:
			intelligence += talent.Value
		case custom_id.FairyTalent_Defense:
			defense += talent.Value
		}
	}
	ends[custom_id.FairyTalent_Energy] = &pb3.FairyEndowment{EnId: custom_id.FairyTalent_Energy, Value: energy}
	ends[custom_id.FairyTalent_Power] = &pb3.FairyEndowment{EnId: custom_id.FairyTalent_Power, Value: power}
	ends[custom_id.FairyTalent_Intelligence] = &pb3.FairyEndowment{EnId: custom_id.FairyTalent_Intelligence, Value: intelligence}
	ends[custom_id.FairyTalent_Defense] = &pb3.FairyEndowment{EnId: custom_id.FairyTalent_Defense, Value: defense}

	return ends
}

func (sys *FairySystem) GetFairyStatus() map[uint64]*pb3.SyncFairyStatus {
	return sys.status
}

func (sys *FairySystem) ResumeAllFairys() {
	for _, sfs := range sys.status {
		if sfs.DieTime > 0 {
			sfs.DieTime = 1
		}
	}
}

func (sys *FairySystem) c2FairyGradeUp(msg *base.Message) error {
	var req pb3.C2S_27_16
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	fairy, err := sys.GetFairy(req.GetHandle())
	if nil != err {
		return err
	}
	fairyConf := jsondata.GetFairyConf(fairy.GetItemId())
	if nil == fairyConf {
		return neterror.ConfNotFoundError("fairyConf conf(%d) is nil", fairy.GetItemId())
	}
	colorMin := jsondata.GlobalUint("fairyColorUpMin")
	if fairyConf.Color < colorMin {
		return neterror.ParamsInvalidError("fairy(%d) upgrade color(%d) not allow", fairy.GetItemId(), fairyConf.Color)
	}
	grade := fairy.Ext.FairyGrade
	if grade > custom_id.FairyGradeSpBegin {
		return neterror.ParamsInvalidError("src god fairy cant grade up")
	}
	ntConf := jsondata.GetFairyGradeUpConfByGrade(grade + 1)
	if nil == ntConf {
		return neterror.ConfNotFoundError("fairy grade up conf(%d) is nil", grade+1)
	}
	if !sys.owner.ConsumeByConf(ntConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairyGradeUp}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	fairy.Ext.FairyGrade = grade + 1
	sys.SyncItemChange(fairy, pb3.LogId_LogFairyStarUp)
	sys.SendProto3(27, 120, &pb3.S2C_27_120{
		Hdl:   fairy.GetHandle(),
		Grade: fairy.Ext.FairyGrade,
	})
	sys.onEndowmentsChange(fairy)
	return nil
}

func (sys *FairySystem) getFairyMainTakeNum() uint32 {
	fairyData := sys.GetData()
	var num uint32
	for i := custom_id.FairyLingZhen_MainBegin; i <= custom_id.FairyLingZhen_MainEnd; i++ {
		hdl := fairyData.BattleFairy[uint32(i)]
		fairy, _ := sys.GetFairy(hdl)
		if nil != fairy {
			num++
		}
	}
	return num
}

func (sys *FairySystem) onFairyLearnSkill(st *pb3.FairyLearnSkillSt) {
	if nil == st {
		return
	}

	fairy, err := sys.GetFairy(st.GetHdl())
	if nil != err {
		return
	}

	if sys.checkG2FSyncFairyAttrs(fairy) {
		sys.owner.CallActorFunc(actorfuncid.FairyLearnSkill, st)
	}
}

func (sys *FairySystem) getEndowments(fairy *pb3.ItemSt) map[uint32]*pb3.FairyEndowment {
	if nil == fairy {
		return nil
	}
	return sys.GetTotalTalentNoAdd(fairy.Handle)
}

func (sys *FairySystem) onEndowmentsChange(fairy *pb3.ItemSt) {
	if sys.checkG2FSyncFairyAttrs(fairy) {
		sys.owner.CallActorFunc(actorfuncid.G2FCalcFairySysProp, &pb3.SyncFairySysAttrs{
			Id:    attrdef.FairyAttrs,
			Attrs: sys.calcFairyAttrs(fairy),
		})
	}
}

func (sys *FairySystem) syncFairyPower(fairy *pb3.ItemSt) {
	singleCalc := attrcalc.GetSingleCalc()
	defer func() {
		singleCalc.Reset()
	}()
	sys.calcFairyAttrCalc(fairy, singleCalc)
	power := attrcalc.CalcByJobConvertAttackAndDefendGetFightValue(singleCalc, int8(sys.owner.GetJob()))
	fairy.Ext.Power = uint64(power)

	fairyBagSys, ok := sys.owner.GetSysObj(sysdef.SiFairyBag).(*FairyBagSystem)
	if !ok {
		sys.LogError("fairy bag sys is nil")
		return
	}
	fairyBagSys.OnItemChange(fairy, 0, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFairyStarUp, NoTips: false})
}

func (sys *FairySystem) GetTotalFairyHistoricalStar() uint32 {
	data := sys.GetData()
	if data == nil {
		return 0
	}

	sum := uint32(0)
	for _, v := range data.HistoricalStar {
		sum += v
	}
	return sum
}

// 计算仙灵属性
func (sys *FairySystem) calcFairyAttrs(fairy *pb3.ItemSt) *pb3.SysAttr {
	singleCalc := attrcalc.GetSingleCalc()
	defer func() {
		singleCalc.Reset()
	}()
	sys.calcFairyAttrCalc(fairy, singleCalc)
	syncAttr := &pb3.SysAttr{Attrs: map[uint32]int64{}}
	singleCalc.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
		syncAttr.Attrs[t] += v
	})

	return syncAttr
}
func (sys *FairySystem) calcFairyAttrCalc(fairy *pb3.ItemSt, calc *attrcalc.FightAttrCalc) {
	// 初始属性
	sys.calcFairyInitAttrs(fairy, calc)
	// 升级属性
	sys.calcFairyLvAttrs(fairy, calc)
	// 资质属性
	sys.calcFairyEndAttrs(fairy, calc)
	// 进化属性
	sys.calcFairyStarAttrs(fairy, calc)

	// 仙灵谱激活属性
	sys.calcFairyCollectActive(fairy, calc)
	// 仙灵图腾
	sys.calcFairyTotemSys(fairy, calc)
	// 仙灵寒武
	sys.calcFairyColdWeaponSys(fairy, calc)
}

// 计算仙灵谱加成属性
func (sys *FairySystem) calcFairyCollectionAttrs() *pb3.SysAttr {
	calc := &attrcalc.FightAttrCalc{}
	data := sys.GetData()
	for id, active := range data.CollectionAttr {
		if !active {
			continue
		}
		conf := jsondata.GetFairyCollectionAttrById(id)
		if nil == conf {
			continue
		}
		for _, line := range conf.Attrs {
			calc.AddValue(line.Type, attrdef.AttrValueAlias(line.Value))
		}
	}
	syncAttr := &pb3.SysAttr{Attrs: map[uint32]int64{}}
	calc.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
		syncAttr.Attrs[t] += v
	})
	return syncAttr
}

// 仙灵初始属性
func (sys *FairySystem) calcFairyInitAttrs(fairy *pb3.ItemSt, calc *attrcalc.FightAttrCalc) {
	fConf := jsondata.GetFairyConf(fairy.GetItemId())
	if fConf != nil {
		for _, line := range fConf.InitAttrs {
			calc.AddValue(line.Type, attrdef.AttrValueAlias(line.Value))
		}
	}
}

// 仙灵等级属性
func (sys *FairySystem) calcFairyLvAttrs(fairy *pb3.ItemSt, calc *attrcalc.FightAttrCalc) {
	lvConf := jsondata.GetFairyLevelConf(fairy.ItemId, fairy.Union1)
	if lvConf != nil {
		for _, line := range lvConf.Attrs {
			calc.AddValue(line.Type, attrdef.AttrValueAlias(line.Value))
		}
		speed := int64(jsondata.GlobalUint("petMoveSpeed"))
		calc.AddValue(attrdef.Speed, speed)

		// 玩家身上拥有仙灵的加成属性
		calc.AddValue(attrdef.MaxHpAddRate, sys.owner.GetFightAttr(attrdef.FairyMaxHpAddRate))
		calc.AddValue(attrdef.AttAddRate, sys.owner.GetFightAttr(attrdef.FairyAttackAddRate))
		calc.AddValue(attrdef.DefAddRate, sys.owner.GetFightAttr(attrdef.FairyDefAddRate))
	}
}

// 仙灵资质属性
func (sys *FairySystem) calcFairyEndAttrs(fairy *pb3.ItemSt, calc *attrcalc.FightAttrCalc) {
	ends := sys.getEndowments(fairy)
	for endId := range ends {
		sys.calcFairyEndAttrsByEndId(fairy, endId, calc)
	}
}

func (sys *FairySystem) calcFairyEndAttrsByEndId(fairy *pb3.ItemSt, endId uint32, calc *attrcalc.FightAttrCalc) {
	eConf := jsondata.GetFairyEndConf(endId)
	if eConf == nil {
		return
	}

	endVal := sys.calcFairyEndValue(fairy, endId)
	tranRate := float64(eConf.TranRate) / custom_id.FairyCoeRate
	attrVal := endVal * tranRate

	if attrVal == 0 {
		return
	}

	for i := range eConf.AttrIds {
		attrId := eConf.AttrIds[i]
		calc.AddValue(attrId, attrdef.AttrValueAlias(attrVal))
	}
}

// 仙灵资质值
func (sys *FairySystem) calcFairyEndValue(fairy *pb3.ItemSt, endId uint32) float64 {
	eConf := jsondata.GetFairyEndConf(endId)
	if eConf == nil {
		return 0
	}

	fConf := jsondata.GetFairyConf(fairy.ItemId)
	if fConf == nil {
		return 0
	}

	ends := sys.getEndowments(fairy)
	if len(ends) == 0 {
		return 0
	}

	value, ok := ends[endId]
	if !ok {
		return 0
	}

	breakLv := sys.getBreakLv(fairy)
	//资质转化属性=（资质数值*系数1+突破等级*系数2）*max(1,系数3) *max(1,品质系数)
	coe1 := sys.getCoe1()
	coe2 := sys.getCoe2(fairy)
	coe3 := sys.getCoe3(fairy)

	value1 := float64(value.Value) * float64(coe1) / custom_id.FairyCoeRate
	value2 := float64(breakLv) * float64(coe2) / custom_id.FairyCoeRate

	rate := utils.MaxFloat64(1, float64(coe3)/custom_id.FairyCoeRate)
	colorRate := utils.MaxFloat64(1, float64(fConf.ColorCoe)/custom_id.FairyCoeRate)
	endVal := (value1 + value2) * rate * colorRate

	return endVal
}

// 仙灵进化属性
func (sys *FairySystem) calcFairyStarAttrs(fairy *pb3.ItemSt, calc *attrcalc.FightAttrCalc) {
	lvConf := jsondata.GetFairyLevelConf(fairy.ItemId, fairy.Union1)
	if lvConf != nil {
		starAddRate := sys.getStarAdd(fairy)
		for _, line := range lvConf.Attrs {
			rate := float64(starAddRate) / custom_id.FairyCoeRate
			value := float64(line.Value) * rate
			calc.AddValue(line.Type, attrdef.AttrValueAlias(value))
		}
	}
}

// 仙灵谱激活属性(出站位的仙灵)
func (sys *FairySystem) calcFairyCollectActive(fairy *pb3.ItemSt, calc *attrcalc.FightAttrCalc) {
	if !itemdef.IsFairyMainPos(fairy.Pos) {
		return
	}
	data := sys.GetData()
	for id, active := range data.CollectionAttr {
		if !active {
			continue
		}
		conf := jsondata.GetFairyCollectionAttrById(id)
		if nil == conf {
			continue
		}
		for _, line := range conf.Attrs {
			calc.AddValue(line.Type, attrdef.AttrValueAlias(line.Value))
		}
	}
}

// 仙灵谱激活属性(出站位的仙灵)
func (sys *FairySystem) calcFairyTotemSys(fairy *pb3.ItemSt, calc *attrcalc.FightAttrCalc) {
	if !itemdef.IsFairyMainPos(fairy.Pos) {
		return
	}

	obj := sys.owner.GetSysObj(sysdef.SiFairyTotem)
	if nil == obj {
		return
	}

	totemSys, ok := obj.(*FairyTotemSys)
	if !ok || !totemSys.IsOpen() {
		return
	}
	totemSys.calcFairyAttrs(calc)
}

// 仙灵寒武激活属性(出站位的仙灵)
func (sys *FairySystem) calcFairyColdWeaponSys(fairy *pb3.ItemSt, calc *attrcalc.FightAttrCalc) {
	if !itemdef.IsFairyMainPos(fairy.Pos) {
		return
	}

	obj := sys.owner.GetSysObj(sysdef.SiFairyColdWeapon)
	if nil == obj {
		return
	}

	s, ok := obj.(*FairyColdWeaponSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcFairyAttrs(calc)
}

// 图鉴系数
func (sys *FairySystem) getCoe1() uint32 {
	collectId := sys.getMaxCollectId()
	conf := jsondata.GetFairyCollectionAttrById(collectId)
	if conf == nil {
		return custom_id.FairyCoeRate
	}
	return conf.Coe
}

// 突破系数
func (sys *FairySystem) getCoe2(fairy *pb3.ItemSt) uint32 {
	conf := jsondata.GetFairyBreakConf(sys.getBreakLv(fairy))
	if conf == nil {
		return 0
	}
	return conf.Coe
}

// 进化系数
func (sys *FairySystem) getCoe3(fairy *pb3.ItemSt) uint32 {
	conf := jsondata.GetFairyStarConf(fairy.ItemId, sys.getStar(fairy))
	if conf == nil {
		return 0
	}
	return conf.Coe
}

// 进化属性加成
func (sys *FairySystem) getStarAdd(fairy *pb3.ItemSt) uint32 {
	conf := jsondata.GetFairyStarConf(fairy.ItemId, sys.getStar(fairy))
	if conf == nil {
		return 0
	}
	return conf.AdditionRate
}

// 获取最大图鉴id
func (sys *FairySystem) getMaxCollectId() uint32 {
	data := sys.GetData()
	var collectId uint32
	for k, v := range data.CollectionAttr {
		if v && k > collectId {
			collectId = k
		}
	}
	return collectId
}

const (
	FairyActiveCollectionAttrStatusClientCanActive = 1
	FairyActiveCollectionAttrStatusClientActive    = 2
)

func (sys *FairySystem) c2sFairyActiveCollectionAttr(msg *base.Message) error {
	var req pb3.C2S_27_161
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	id := req.GetId()
	conf := jsondata.GetFairyCollectionAttrById(id)
	if nil == conf {
		return neterror.ConfNotFoundError("fairy collection attr conf %d is nil", req.GetId())
	}
	data := sys.GetData()
	status, canActive := data.CollectionAttr[id]
	if !canActive || status {
		return neterror.ParamsInvalidError("fairy collection attr %d cant active status", req.GetId())
	}
	data.CollectionAttr[id] = true
	sys.owner.SendProto3(27, 161, &pb3.S2C_27_161{
		Id:     id,
		Stauts: FairyActiveCollectionAttrStatusClientActive,
	})

	sys.reCalcFairyMainPower()

	sys.owner.TriggerEvent(custom_id.AeReCalcFairyMainPower)
	return nil
}

func (sys *FairySystem) c2sGetFairyEquips(_ *base.Message) error {
	sys.S2CFairyEquips()
	return nil
}

func (sys *FairySystem) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_27_45
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		sys.LogError("err:%v", err)
		return err
	}

	pos, err := sys.checkTakeOn(req.Slot, req.Hdl)
	if err != nil {
		sys.LogError("err: %v", err)
		return err
	}

	sys.takeOn(req.Slot, pos, req.Hdl)
	return nil
}

func (sys *FairySystem) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_27_46
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		sys.LogError("err:%v", err)
		return err
	}

	if err = sys.checkSlotValid(req.Slot); err != nil {
		sys.LogError("err:%v", err)
		return err
	}

	sys.takeOff(req.Slot, req.Pos)
	return nil
}

func (sys *FairySystem) c2sFastTakeOn(msg *base.Message) error {
	var req pb3.C2S_27_47
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		sys.LogError("err:%v", err)
		return err
	}

	hdlPos := make(map[uint64]uint32)
	for _, hdl := range req.HdlL {
		pos, err := sys.checkTakeOn(req.Slot, hdl)
		if err != nil {
			sys.LogError("err: %v", err)
			return err
		}
		hdlPos[hdl] = pos
	}

	for hdl, pos := range hdlPos {
		sys.takeOn(req.Slot, pos, hdl)
	}

	sys.pushFairyEquip(req.Slot)
	return nil
}

func (sys *FairySystem) c2sFastTakeOff(msg *base.Message) error {
	var req pb3.C2S_27_48
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		sys.LogError("err:%v", err)
		return err
	}

	count := sys.GetOwner().GetFairyBagAvailableCount()
	if count < uint32(len(itemdef.FEquipPosTypeMap)) {
		sys.GetOwner().SendTipMsg(tipmsgid.TpFairyEquipBagIsFull)
		return nil
	}

	if err = sys.checkSlotValid(req.Slot); err != nil {
		sys.LogError("err:%v", err)
		return err
	}

	data := sys.GetEquipData(req.Slot)

	var posL []uint32
	for pos := range data.PosData {
		posL = append(posL, pos)
	}
	for _, pos := range posL {
		sys.takeOff(req.Slot, pos)
	}

	sys.pushFairyEquip(req.Slot)
	return nil
}

func (sys *FairySystem) c2sSrcGodTakeOn(msg *base.Message) error {
	var req pb3.C2S_27_88
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if sys.owner.GetFightStatus() {
		sys.owner.SendTipMsg(tipmsgid.TpFightingLimit)
		return nil
	}

	pos := req.Pos

	if !itemdef.IsFairyMainPos(pos) {
		return neterror.ParamsInvalidError("pos:%d is not main", pos)
	}

	fairy := sys.GetFairyByBattlePos(pos)
	if fairy == nil {
		return neterror.ParamsInvalidError("fairy not found, battle pos:%d", pos)
	}

	sys.srcGodTakeOn(pos, req.Id)
	sys.SendProto3(27, 88, &pb3.S2C_27_88{Id: req.Id, Pos: pos})
	return nil
}

func (sys *FairySystem) c2sSrcGodTakeOff(msg *base.Message) error {
	var req pb3.C2S_27_89
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if sys.owner.GetFightStatus() {
		sys.owner.SendTipMsg(tipmsgid.TpFightingLimit)
		return nil
	}

	pos := req.Pos

	if !itemdef.IsFairyMainPos(pos) {
		return neterror.ParamsInvalidError("pos:%d is not main", pos)
	}

	fairy := sys.GetFairyByBattlePos(pos)
	if fairy == nil {
		return neterror.ParamsInvalidError("fairy not found, battle pos:%d", pos)
	}

	sys.srcGodTakeOff(pos)
	sys.SendProto3(27, 89, &pb3.S2C_27_89{Pos: pos})
	return nil
}

func (sys *FairySystem) srcGodTakeOn(pos, id uint32) {
	srcGod := sys.GetSrcGodByBattlePos(pos)
	// 卡槽有源神
	if srcGod > 0 {
		sys.srcGodTakeOff(pos)
	}
	// 自己本身在卡槽
	otherPos := sys.GetBattlePosBySrcGod(id)
	if otherPos > 0 {
		sys.srcGodTakeOff(otherPos)
	}

	fairyData := sys.GetData()
	fairyData.BattleSrcGod[pos] = id
	sys.aftersSrcGodTakeOn(pos)
}

func (sys *FairySystem) srcGodTakeOff(pos uint32) {
	fairyData := sys.GetData()
	fairyData.BattleSrcGod[pos] = 0
	sys.aftersSrcGodTakeOff(pos)
}

func (sys *FairySystem) setFairyExtraAttr(pos uint32, attrType uint32, attrVal int64) {
	fairy := sys.GetFairyByBattlePos(pos)
	if nil == fairy {
		return
	}
	err := sys.owner.CallActorFunc(actorfuncid.SyncFairyExtraAttr, &pb3.SyncFairyExtraAttr{
		Hdl:      fairy.GetHandle(),
		AttrType: attrType,
		AttrVal:  attrVal,
	})
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

func (sys *FairySystem) aftersSrcGodTakeOn(pos uint32) {
	sys.setFairyExtraAttr(pos, attrdef.SourceGodTakeId, int64(sys.GetData().BattleSrcGod[pos]))
}

func (sys *FairySystem) aftersSrcGodTakeOff(pos uint32) {
	sys.setFairyExtraAttr(pos, attrdef.SourceGodTakeId, int64(sys.GetData().BattleSrcGod[pos]))
}

func (sys *FairySystem) checkTakeOn(slot uint32, hdl uint64) (uint32, error) {
	st := sys.GetOwner().GetFairyEquipItemByHandle(hdl)
	if st == nil {
		return 0, neterror.ParamsInvalidError("not found item, hdl:%d", hdl)
	}

	itemConf := jsondata.GetItemConfig(st.ItemId)
	if itemConf == nil {
		return 0, neterror.ConfNotFoundError("not found item, id:%d", st.ItemId)
	}

	if !itemdef.IsFairyEquip(itemConf.Type) {
		return 0, neterror.ParamsInvalidError("is not fairy equip, id:%d", st.ItemId)
	}
	if !itemdef.IsFairyEquipSubType(itemConf.SubType) {
		return 0, neterror.ParamsInvalidError("is not fairy equip, id:%d", st.ItemId)
	}

	sConf := jsondata.GetFEquipSlotConf(itemConf.SubType)
	if sConf == nil {
		return 0, neterror.ConfNotFoundError("fairy equip pos not found, id:%d", st.ItemId)
	}

	if err := sys.checkSlotValid(slot); err != nil {
		return 0, err
	}

	fairy := sys.GetFairyByBattlePos(slot)
	if fairy == nil {
		return 0, neterror.ParamsInvalidError("fairy not found, battle pos:%d", slot)
	}

	if fairy.Union1 < sConf.FairyLv {
		return 0, neterror.ParamsInvalidError("slot lv limit, slot:%d", sConf.Pos)
	}

	if fairy.Union2 < sConf.FairyStar {
		return 0, neterror.ParamsInvalidError("slot star limit, slot:%d", sConf.Pos)
	}

	return sConf.Pos, nil
}

func (sys *FairySystem) checkSlotValid(slot uint32) error {
	fairyData := sys.GetData()
	if !utils.SliceContainsUint32(fairyData.GetPos(), slot) {
		return neterror.ParamsInvalidError("slot not open, slot:%d", slot)
	}
	if !itemdef.IsFairyMainPos(slot) {
		return neterror.ParamsInvalidError("is not main slot, slot:%d", slot)
	}
	return nil
}

func (sys *FairySystem) isStarUp(fairy *pb3.ItemSt) bool {
	conf := jsondata.GetFairyConf(fairy.ItemId)
	if conf == nil {
		return false
	}
	if fairy.Union2 > conf.Star {
		return true
	}
	return false
}

// 升级||突破||洗髓
func (sys *FairySystem) isCultivate(fairy *pb3.ItemSt) bool {
	return fairy.Union1 > 1 || fairy.Ext.FairyBackNum > 0
}

func (sys *FairySystem) takeOn(slot, pos uint32, hdl uint64) {
	equipData := sys.GetEquipData(slot)
	// 该位置已穿戴在slot的其他pos上
	if equipData.PosData[pos] > 0 {
		sys.takeOff(slot, pos)
	}

	itemSt := sys.owner.GetFairyEquipItemByHandle(hdl)
	// 该装备穿戴在其他slot
	ownerId := itemSt.Ext.OwnerId
	if ownerId > 0 && ownerId != uint64(slot) {
		sys.takeOff(uint32(ownerId), pos)
	}

	// 穿戴装备
	itemSt.Pos = pos                  // 记录仙灵装备穿在哪个装备位置
	itemSt.Ext.OwnerId = uint64(slot) // 记录仙灵装备穿在哪个上阵孔位
	equipData.PosData[pos] = hdl      // pos: 装备槽 hdl: 装备hdl

	sys.afterTakeOn()
	sys.SendProto3(27, 45, &pb3.S2C_27_45{
		Slot:   slot,
		Pos:    pos,
		ItemSt: sys.owner.GetFairyEquipItemByHandle(hdl),
	})
	sys.SyncFairyEquip(itemSt, pb3.LogId_LogFairyEquipTakeOn)
	sys.owner.TriggerQuestEventRange(custom_id.QttFairyEquipSuitNum)
}

func (sys *FairySystem) takeOff(slot, pos uint32) {
	equipData := sys.GetEquipData(slot)
	hdl := equipData.PosData[pos]
	if hdl == 0 {
		return
	}
	itemSt := sys.owner.GetFairyEquipItemByHandle(hdl)
	itemSt.Pos = 0
	itemSt.Ext.OwnerId = 0
	equipData.PosData[pos] = 0
	sys.afterTakeOff()
	sys.SendProto3(27, 46, &pb3.S2C_27_46{
		Slot: slot,
		Pos:  pos,
	})
	sys.SyncFairyEquip(itemSt, pb3.LogId_LogFairyEquipTakeOff)
}

func (sys *FairySystem) pushFairyEquip(slot uint32) {
	fairyEquip := make(map[uint32]*pb3.FairyEquipData)
	fairyEquip[slot] = sys.GetEquipData(slot)
	sys.SendProto3(27, 49, &pb3.S2C_27_49{
		Data: fairyEquip,
	})
}

func (sys *FairySystem) getCalcExp(hdlMap map[uint64]uint32) (uint32, []uint64) {
	var exp uint32
	var hdlL []uint64
	for hdl, count := range hdlMap {
		itemSt := sys.owner.GetFairyEquipItemByHandle(hdl)
		if itemSt == nil {
			continue
		}
		itemConf := jsondata.GetItemConfig(itemSt.ItemId)
		switch itemConf.Type {
		case itemdef.ItemTypeFairyEquip:
			conf := jsondata.GetFEquipConf(itemSt.ItemId)
			if conf == nil {
				continue
			}
			exp += conf.BaseExp
			hdlL = append(hdlL, hdl)
			lConf := jsondata.GetFEquipEnhanceConf(itemSt.ItemId, itemSt.Union1)
			if lConf != nil {
				rateExp := utils.CalcMillionRate64(int64(lConf.ConsumeExp), int64(conf.Rate))
				exp += uint32(rateExp)
			}
		case itemdef.ItemTypeFairyEquipMaterials:
			exp += itemConf.CommonField * count
			hdlL = append(hdlL, hdl)
		}
	}

	return exp, hdlL
}

func (sys *FairySystem) afterTakeOn() {
	sys.ResetSysAttr(attrdef.SaFairyEquip)
}

func (sys *FairySystem) afterTakeOff() {
	sys.ResetSysAttr(attrdef.SaFairyEquip)
}

func (sys *FairySystem) reCalcFairyMainPower() {
	// 同步出站位仙灵战力
	data := sys.GetData()
	for pos, hdl := range data.BattleFairy {
		if !itemdef.IsFairyMainPos(pos) {
			continue
		}
		fairy, _ := sys.GetFairy(hdl)
		if fairy == nil {
			continue
		}
		sys.syncFairyPower(fairy)

		// 同步战斗仙灵的属性到战斗服
		if sys.checkG2FSyncFairyAttrs(fairy) {
			sys.owner.CallActorFunc(actorfuncid.G2FCalcFairySysProp, &pb3.SyncFairySysAttrs{
				Id:    attrdef.FairyAttrs,
				Attrs: sys.calcFairyAttrs(fairy),
			})
		}
	}

	sys.ResetSysAttr(attrdef.FairySeat)
}

func (sys *FairySystem) checkFairyCollectionAttr() {
	if nil == sys.owner.GetMainData().ItemPool {
		return
	}
	fairyBag := sys.owner.GetMainData().ItemPool.FairyBag
	colorStar := make(map[uint64]uint32)
	for _, itemSt := range fairyBag {
		fairyConf := jsondata.GetFairyConf(itemSt.GetItemId())
		if nil == fairyConf {
			continue
		}
		colorStar[utils.Make64(fairyConf.Color, itemSt.Union2)]++
	}
	confList := jsondata.GetFairyCollectionAttr()
	data := sys.GetData()
	for id, conf := range confList {
		_, changed := data.CollectionAttr[id]
		if changed {
			continue
		}
		var count uint32
		for key, val := range colorStar {
			color, star := utils.Low32(key), utils.High32(key)
			if color < conf.Color || star < conf.Star {
				continue
			}
			count += val
		}
		if count < conf.Count {
			continue
		}
		data.CollectionAttr[id] = false
		sys.owner.SendProto3(27, 161, &pb3.S2C_27_161{
			Id:     id,
			Stauts: FairyActiveCollectionAttrStatusClientCanActive,
		})
	}
}

func (sys *FairySystem) SyncItemChange(fairy *pb3.ItemSt, logId pb3.LogId) {
	fairyBagSys, ok := sys.owner.GetSysObj(sysdef.SiFairyBag).(*FairyBagSystem)
	if !ok {
		sys.LogError("fairy bag sys is nil")
		return
	}

	// 仙灵战力
	singleCalc := attrcalc.GetSingleCalc()
	defer func() {
		singleCalc.Reset()
	}()
	sys.calcFairyAttrCalc(fairy, singleCalc)

	player := sys.GetOwner()
	power := attrcalc.CalcByJobConvertAttackAndDefendGetFightValue(singleCalc, int8(player.GetJob()))
	fairy.Ext.Power = uint64(power)
	fairyBagSys.OnItemChange(fairy, 0, common.EngineGiveRewardParam{LogId: logId, NoTips: false})

	// 仙灵星级属性直接给玩家加战力
	sys.ResetSysAttr(attrdef.FairyStarProp)
	sys.ResetSysAttr(attrdef.FairySeat)
}

func (sys *FairySystem) SyncFairyEquip(fEquip *pb3.ItemSt, logId pb3.LogId) {
	fairyEquipSys, ok := sys.owner.GetSysObj(sysdef.SiFairyEquip).(*FairyEquipSys)
	if ok && fairyEquipSys.IsOpen() {
		fairyEquipSys.SyncItemChange(fEquip, logId)
	}
}

func (sys *FairySystem) checkDieAll() bool {
	data := sys.GetData()
	battleFairy := data.BattleFairy
	if data.BattleFairy == nil {
		return false
	}
	var flag = true
	for i := custom_id.FairyLingZhen_MainBegin; i <= custom_id.FairyLingZhen_MainEnd; i++ {
		_, ok := battleFairy[uint32(i)]
		if !ok {
			continue
		}
		if !sys.IsDeath(uint32(i)) {
			flag = false
			break
		}
	}
	return flag
}

func (sys *FairySystem) getFightValue() int64 {
	data := sys.GetData()
	var fightValue int64
	for _, val := range data.BattleFairy {
		fairy, err := sys.GetFairy(val)
		if err != nil {
			sys.LogError("err:%v", err)
			continue
		}
		if fairy == nil || fairy.Ext == nil {
			continue
		}
		fightValue += int64(fairy.Ext.Power)
	}
	return fightValue
}

func (sys *FairySystem) getTakenOnFairyCount(lzType uint32) uint32 {
	fData := sys.GetData()
	var count uint32
	for pos, _ := range fData.BattleFairy {
		lzConf := jsondata.GetFairyLingZhenConf(pos)
		if lzConf != nil && lzConf.ZhenType == lzType {
			count++
		}
	}
	return count
}

func checkFairyCollectionAttr(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if !ok {
		return
	}
	sys.checkFairyCollectionAttr()
	player.TriggerQuestEventRange(custom_id.QttFairyQualityNum)
}

func handleReCalcFairyMainPower(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if !ok {
		return
	}
	sys.reCalcFairyMainPower()
}

func calcFairySeatAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	calcFairyMainSeatAttr(player, calc)
	calcFairySupSeatAttr(player, calc)
}

func calcFairyStarAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if !ok || !sys.IsOpen() {
		return
	}
	fairyData := sys.GetData()
	for _, hdl := range fairyData.BattleFairy {
		// 获取仙灵属性
		fairy, _ := sys.GetFairy(hdl)
		if fairy == nil {
			continue
		}
		fairyStarConf := jsondata.GetFairyStarConf(fairy.ItemId, fairy.Union2)
		if fairyStarConf == nil {
			continue
		}
		engine.CheckAddAttrsToCalc(player, calc, fairyStarConf.Attrs1)
	}
}

// 出站仙灵增加属性 = 仙灵属性 * Ratio
func calcFairyMainSeatAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if !ok || !sys.IsOpen() {
		return
	}
	fairyData := sys.GetData()
	for pos, hdl := range fairyData.BattleFairy {
		lzConf := jsondata.GetFairyLingZhenConf(pos)
		if nil != lzConf && lzConf.ZhenType == custom_id.FairyLingZhen_TypeMain && hdl > 0 {
			// 获取仙灵
			fairy, _ := sys.GetFairy(hdl)
			if fairy == nil {
				continue
			}

			// 转化玩家战力
			value := fairy.GetExt().Power
			attrs := jsondata.AttrVec{&jsondata.Attr{Type: attrdef.FairyMagicPower, Value: uint32(value)}}
			engine.CheckAddAttrsToCalc(player, calc, attrs)
		}
	}
}

// 助战宠物增加属性 = 出站仙灵 * EndowType
func calcFairySupSeatAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if !ok || !sys.IsOpen() {
		return
	}
	fairyData := sys.GetData()
	for pos, hdl := range fairyData.BattleFairy {
		lzConf := jsondata.GetFairyLingZhenConf(pos)
		if nil != lzConf && lzConf.ZhenType == custom_id.FairyLingZhen_TypeSupport && hdl > 0 {
			// 获取仙灵对应资质属性
			fairy, _ := sys.GetFairy(hdl)
			if fairy == nil {
				continue
			}
			fCalc := &attrcalc.FightAttrCalc{}
			for _, endId := range lzConf.EndowType {
				sys.calcFairyEndAttrsByEndId(fairy, endId, fCalc)
			}

			// 转换玩家属性加成
			var attrs jsondata.AttrVec
			fCalc.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
				if v > 0 {
					ty := t
					if t2 := jsondata.GetFairy2PlayerAttr(t); t2 > 0 {
						ty = t2
					}
					attrs = append(attrs, &jsondata.Attr{Type: ty, Value: uint32(v)})
				}
			})
			engine.CheckAddAttrsToCalc(player, calc, attrs)
		}
	}
}

func calFairyCollection(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	fairySys := player.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if nil == fairySys || !fairySys.IsOpen() {
		return
	}
	data := fairySys.GetData()
	for itemId, star := range data.Collection {
		starConf := jsondata.GetFairyStarConf(itemId, star)
		if nil != starConf {
			engine.CheckAddAttrsToCalc(player, calc, starConf.Attrs)
		}
	}
}

func GmFairyLvUp(actor iface.IPlayer, args ...string) bool {
	sys, ok := actor.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if !ok {
		return false
	}
	hdl := utils.AtoUint64(args[0])
	lv := utils.AtoUint32(args[1])
	if fairy, _ := sys.GetFairy(hdl); nil != fairy {
		fairy.Union1 = lv
		sys.SyncItemChange(fairy, pb3.LogId_LogFairyLvUp)
		sys.SendProto3(27, 8, &pb3.S2C_27_8{Handle: hdl, Lv: lv})
		if sys.checkG2FSyncFairyAttrs(fairy) {
			sys.owner.CallActorFunc(actorfuncid.SyncFairyLv, &pb3.CommonSt{
				U64Param: fairy.GetHandle(),
				U32Param: fairy.GetUnion1(),
			})
		}
	}

	return true
}

// 任意/指定仙灵最高等级
func fairyLvQuestTarget(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	var needFairyItemId = uint32(0)
	if len(ids) >= 1 {
		needFairyItemId = ids[0]
	}
	var maxFairyLv uint32
	for _, itemSt := range actor.GetMainData().ItemPool.FairyBag {
		if (needFairyItemId == 0 || needFairyItemId == itemSt.ItemId) && maxFairyLv < itemSt.Union1 { // 如果=0表示任意, > 0 指定仙灵(道具)
			maxFairyLv = itemSt.Union1
		}
	}
	return maxFairyLv
}

// 任意/指定仙灵最高星数(进化)
func fairyStarQuestTarget(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	var needFairyItemId = uint32(0)
	if len(ids) >= 1 {
		needFairyItemId = ids[0]
	}
	var maxFairyStar uint32
	for _, itemSt := range actor.GetMainData().ItemPool.FairyBag {
		if (needFairyItemId == 0 || needFairyItemId == itemSt.ItemId) && maxFairyStar < itemSt.Union2 { // 如果=0表示任意, > 0 指定仙灵(道具)
			maxFairyStar = itemSt.Union2
		}
	}
	return maxFairyStar
}

// 激活X只X品质仙灵
func fairyQualityQuestTarget(actor iface.IPlayer, ids []uint32, args ...any) uint32 {
	if len(ids) <= 0 {
		return 0
	}
	needFairyQuality := ids[0]
	sys, ok := actor.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if !ok || !sys.IsOpen() {
		return 0
	}
	var count uint32
	for _, itemSt := range actor.GetMainData().ItemPool.FairyBag {
		fairyConf := jsondata.GetFairyConf(itemSt.ItemId)
		if fairyConf == nil {
			continue
		}
		if fairyConf.Color >= needFairyQuality {
			count += 1
		}
	}
	return count
}

func fairyStarNumQuestTarget(actor iface.IPlayer, ids []uint32, args ...any) uint32 {
	if len(ids) <= 0 {
		return 0
	}
	needStar := ids[0]
	var count uint32
	for _, itemSt := range actor.GetMainData().ItemPool.FairyBag {
		if itemSt.Union2 >= needStar {
			count++
		}
	}
	return count
}

func fairyTakeOnCountTarget(actor iface.IPlayer, ids []uint32, args ...any) uint32 {
	if len(ids) <= 0 {
		return 0
	}

	sys, ok := actor.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if !ok || !sys.IsOpen() {
		return 0
	}

	lzType := ids[0]
	count := sys.getTakenOnFairyCount(lzType)
	return count
}

func ResumeAllFairys(player iface.IPlayer, _ []byte) {
	fairySys := player.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if fairySys == nil || !fairySys.IsOpen() {
		return
	}

	fairySys.ResumeAllFairys()
}

func GmFairyCollectionAttr(actor iface.IPlayer, args ...string) bool {
	sys, ok := actor.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if !ok {
		return false
	}
	data := sys.GetData()
	conf := jsondata.GetFairyCollectionAttr()
	for _, v := range conf {
		data.CollectionAttr[v.Id] = true
	}
	sys.SendProto3(27, 160, &pb3.S2C_27_160{Ids: data.CollectionAttr})

	sys.reCalcFairyMainPower()
	return true
}

func handlePowerRushRankSubTypeFairy(player iface.IPlayer) (score int64) {
	attrSys := player.GetAttrSys()
	if attrSys == nil {
		return 0
	}
	var totalPower int64
	var sysIds = []uint32{
		attrdef.FairySeat,
		attrdef.FairyStarProp,
		attrdef.FairyCollectionProperty,
		attrdef.SaFairyMagic,
		attrdef.SaFairyEquip,
		attrdef.SaFairyTotem,
		attrdef.SaFairyColdWeapon,
		attrdef.SaFairySpirit,
		attrdef.SaFairySpiritSuit,
		attrdef.SaSourceGod,
	}
	for _, sysId := range sysIds {
		totalPower += attrSys.GetSysPower(sysId)
	}
	return totalPower
}

func handlePlayScoreRankTypeFairy(player iface.IPlayer) *pb3.YYFightValueRushRankExt {
	sys, ok := player.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if !ok || !sys.IsOpen() {
		return nil
	}
	var ext = &pb3.YYFightValueRushRankExt{}
	ext.FairyMgr = []*pb3.YYFightValueRushRankExtFairy{}
	for pos, hdl := range sys.GetData().BattleFairy {
		if !itemdef.IsFairyMainPos(pos) {
			continue
		}
		fairy, _ := sys.GetFairy(hdl)
		if fairy != nil {
			ext.FairyMgr = append(ext.FairyMgr, &pb3.YYFightValueRushRankExtFairy{
				ItemId: fairy.ItemId,
				Lv:     fairy.Union1,
				Star:   fairy.Union2,
			})
		}
	}
	return ext
}

func fairyOnUpdateSysPowerMap(player iface.IPlayer, args ...interface{}) {
	fairySys := player.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if nil == fairySys || !fairySys.IsOpen() {
		return
	}
	data := fairySys.GetData()
	var sumPower int64
	for _, val := range data.BattleFairy {
		fairy, err := fairySys.GetFairy(val)
		if err != nil {
			player.LogError("err:%v", err)
			continue
		}
		if fairy == nil || fairy.Ext == nil {
			continue
		}
		sumPower += int64(fairy.Ext.Power)
	}
	manager.UpdatePlayScoreRank(ranktype.PlayScoreRankTypeFairy, player, sumPower, false, 0)
}

func init() {
	RegisterSysClass(sysdef.SiFairy, func() iface.ISystem {
		return &FairySystem{}
	})

	event.RegActorEvent(custom_id.AeLoginFight, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiFairy).(*FairySystem)
		if !ok || !sys.IsOpen() {
			return
		}
		if sys.battlePos > 0 {
			sys.CallFairyToBattle(sys.battlePos, false)
		}
	})

	event.RegActorEvent(custom_id.AeGetNewFairy, checkFairyCollectionAttr)

	event.RegActorEvent(custom_id.AeReCalcFairyMainPower, handleReCalcFairyMainPower)

	engine.RegQuestTargetProgress(custom_id.QttTakeOnFairyMainNum, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		sys, ok := actor.GetSysObj(sysdef.SiFairy).(*FairySystem)
		if !ok || !sys.IsOpen() {
			return 0
		}
		return sys.getFairyMainTakeNum()
	})

	gmevent.Register("fairy.lvUp", GmFairyLvUp, 1)
	gmevent.Register("fairy.collectionAttr", GmFairyCollectionAttr, 1)

	engine.RegAttrCalcFn(attrdef.FairySeat, calcFairySeatAttr)
	engine.RegAttrCalcFn(attrdef.FairyCollectionProperty, calFairyCollection)
	engine.RegAttrCalcFn(attrdef.FairyStarProp, calcFairyStarAttr)

	engine.RegisterActorCallFunc(playerfuncid.SyncFairySkillCd, syncSkillCd)
	engine.RegisterActorCallFunc(playerfuncid.SyncFairyStatus, syncStatus)
	engine.RegisterActorCallFunc(playerfuncid.CheckAndCallFairyToBattle, F2GTryReCallFairyToBattle)
	engine.RegisterActorCallFunc(playerfuncid.CallLastBattleFairyOut, CallLastBattleFairyOut)

	engine.RegisterActorCallFunc(playerfuncid.ResumeAllFairy, ResumeAllFairys)

	net.RegisterSysProto(27, 5, sysdef.SiFairy, (*FairySystem).c2sUnLockSlot)
	net.RegisterSysProto(27, 6, sysdef.SiFairy, (*FairySystem).c2sFairyIntoSlot)
	net.RegisterSysProto(27, 7, sysdef.SiFairy, (*FairySystem).c2sFairyIntoBag)
	net.RegisterSysProto(27, 8, sysdef.SiFairy, (*FairySystem).c2sLevelUp)

	net.RegisterSysProto(27, 11, sysdef.SiFairy, (*FairySystem).c2sCollect)
	net.RegisterSysProto(27, 12, sysdef.SiFairy, (*FairySystem).c2sFairyBack)
	net.RegisterSysProto(27, 13, sysdef.SiFairy, (*FairySystem).c2sTalentRefresh)
	net.RegisterSysProto(27, 14, sysdef.SiFairy, (*FairySystem).c2sTalentCheck)
	net.RegisterSysProto(27, 15, sysdef.SiFairy, (*FairySystem).c2sStarUp)
	net.RegisterSysProto(27, 16, sysdef.SiFairy, (*FairySystem).c2sBreak)

	net.RegisterSysProto(27, 44, sysdef.SiFairy, (*FairySystem).c2sGetFairyEquips)
	net.RegisterSysProto(27, 45, sysdef.SiFairy, (*FairySystem).c2sTakeOn)
	net.RegisterSysProto(27, 46, sysdef.SiFairy, (*FairySystem).c2sTakeOff)
	net.RegisterSysProto(27, 47, sysdef.SiFairy, (*FairySystem).c2sFastTakeOn)
	net.RegisterSysProto(27, 48, sysdef.SiFairy, (*FairySystem).c2sFastTakeOff)

	net.RegisterSysProto(27, 50, sysdef.SiFairy, (*FairySystem).c2sFairyToBattle)
	net.RegisterSysProto(27, 65, sysdef.SiFairy, (*FairySystem).c2sSendLastBattle)

	net.RegisterSysProto(27, 88, sysdef.SiFairy, (*FairySystem).c2sSrcGodTakeOn)
	net.RegisterSysProto(27, 89, sysdef.SiFairy, (*FairySystem).c2sSrcGodTakeOff)

	net.RegisterSysProto(27, 120, sysdef.SiFairy, (*FairySystem).c2FairyGradeUp)
	net.RegisterSysProto(27, 161, sysdef.SiFairy, (*FairySystem).c2sFairyActiveCollectionAttr)

	engine.RegQuestTargetProgress(custom_id.QttFairyStarMax, fairyStarQuestTarget)
	engine.RegQuestTargetProgress(custom_id.QttFairyQualityNum, fairyQualityQuestTarget)
	engine.RegQuestTargetProgress(custom_id.QttFairyLvMax, fairyLvQuestTarget)
	engine.RegQuestTargetProgress(custom_id.QttFairyStarNum, fairyStarNumQuestTarget)
	engine.RegQuestTargetProgress(custom_id.QttTakeOnFairy, fairyTakeOnCountTarget)

	manager.RegPlayScoreExtValueGetter(ranktype.PlayScoreRankTypeFairy, handlePlayScoreRankTypeFairy)

	event.RegActorEvent(custom_id.AeUpdateSysPowerMap, fairyOnUpdateSysPowerMap)

	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeFairy, handlePowerRushRankSubTypeFairy)

	gmevent.Register("resetFairyCollect", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiFairy).(*FairySystem)
		if !ok || !sys.IsOpen() {
			return false
		}
		data := sys.GetData()
		data.CollectionAttr = make(map[uint32]bool)
		for id := range jsondata.FairyCollectionAttrConfMgr {
			data.CollectionAttr[id] = false
		}
		return true
	}, 1)
}
