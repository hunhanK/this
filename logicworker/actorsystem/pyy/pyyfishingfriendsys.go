/**
 * @Author:
 * @Date:
 * @Desc:
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/random"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
	"sort"
)

type FishingFriendSys struct {
	PlayerYYBase
}

func (s *FishingFriendSys) s2cInfo() {
	s.SendProto3(8, 240, &pb3.S2C_8_240{
		ActiveId: s.Id,
		Data:     s.getData(),
	})
}

func (s *FishingFriendSys) getData() *pb3.PYYFishingFriendData {
	state := s.GetYYData()
	if nil == state.FishingFriendData {
		state.FishingFriendData = make(map[uint32]*pb3.PYYFishingFriendData)
	}
	if state.FishingFriendData[s.Id] == nil {
		state.FishingFriendData[s.Id] = &pb3.PYYFishingFriendData{}
	}
	if state.FishingFriendData[s.Id].PictureGuideMap == nil {
		state.FishingFriendData[s.Id].PictureGuideMap = make(map[uint32]*pb3.PYYFishingPictureGuide)
	}
	return state.FishingFriendData[s.Id]
}

func (s *FishingFriendSys) getFishingPictureGuide(groundId uint32) *pb3.PYYFishingPictureGuide {
	data := s.getData()
	if data.PictureGuideMap == nil {
		data.PictureGuideMap = make(map[uint32]*pb3.PYYFishingPictureGuide)
	}
	if data.PictureGuideMap[groundId] == nil {
		data.PictureGuideMap[groundId] = &pb3.PYYFishingPictureGuide{}
	}
	if data.PictureGuideMap[groundId].FishMap == nil {
		data.PictureGuideMap[groundId].FishMap = make(map[uint32]*pb3.PYYFishingFish)
	}
	data.PictureGuideMap[groundId].GroundId = groundId
	return data.PictureGuideMap[groundId]
}

func (s *FishingFriendSys) ResetData() {
	state := s.GetYYData()
	if nil == state.FishingFriendData {
		return
	}
	delete(state.FishingFriendData, s.Id)
}

func (s *FishingFriendSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FishingFriendSys) Login() {
	s.s2cInfo()
}

func (s *FishingFriendSys) OnOpen() {
	s.s2cInfo()
}

func (s *FishingFriendSys) drawFish(groundId, fishhookItemId, baitItemId uint32) (*pb3.PYYFishingRet, *jsondata.StdReward, error) {
	var ret = &pb3.PYYFishingRet{}
	config := jsondata.GetPyyFishFriendConfig(s.ConfName, s.ConfIdx)
	if config == nil {
		return nil, nil, neterror.ConfNotFoundError("config not found")
	}
	// 上报钓鱼次数
	s.GetPlayer().TriggerEvent(custom_id.AeAddActivityHandBookExp, int(config.TimesReportYYId), uint32(1))
	var fishingGroundConf *jsondata.FishFriendFishingGround
	for _, ground := range config.FishingGround {
		if ground.Id != groundId {
			continue
		}
		fishingGroundConf = ground
		break
	}
	if fishingGroundConf == nil {
		return nil, nil, neterror.ConfNotFoundError("fishingGroundConf %d not found", groundId)
	}
	data := s.getData()
	if data.Gram < fishingGroundConf.Condition {
		return nil, nil, neterror.ParamsInvalidError("gram not enough %d %d", data.Gram, fishingGroundConf.Condition)
	}

	// step1 检查能否上钩
	baseFishingRate := config.BaseFishingRate
	var fishHookConf *jsondata.FishFriendFishhook
	var baitConf *jsondata.FishFriendBait
	for _, fishhook := range config.Fishhook {
		if fishhook.ItemId != fishhookItemId {
			continue
		}
		fishHookConf = fishhook
		break
	}
	if fishHookConf == nil {
		return nil, nil, neterror.ParamsInvalidError("fishHookConf %d not found", fishhookItemId)
	}
	for _, bait := range config.Bait {
		if bait.ItemId != baitItemId {
			continue
		}
		baitConf = bait
		break
	}
	baseFishingRate += fishHookConf.FishWeight
	if baitConf != nil {
		baseFishingRate += baitConf.FishWeight
	}
	if !random.Hit(baseFishingRate, 10000) {
		ret.IsFailed = true
		return ret, nil, nil
	}
	var randomPool = new(random.Pool)

	// step2 鱼的尺寸
	for _, fishSize := range fishingGroundConf.FishSize {
		weight := fishSize.Weight
		typ := fishSize.Type
		// 鱼钩加成
		sizeWeight, ok := fishHookConf.SizeWeight[typ]
		if ok {
			if sizeWeight.Opt == 0 {
				weight += sizeWeight.AddRate
			} else if sizeWeight.Opt == 1 {
				if weight > sizeWeight.AddRate {
					weight = 0
				} else {
					weight += sizeWeight.AddRate
				}
			}
		}

		// 鱼饵加成
		if baitConf != nil {
			sizeWeight, ok = baitConf.SizeWeight[typ]
			if ok {
				if sizeWeight.Opt == 0 {
					weight += sizeWeight.AddRate
				} else if sizeWeight.Opt == 1 {
					if weight > sizeWeight.AddRate {
						weight = 0
					} else {
						weight += sizeWeight.AddRate
					}
				}
			}
		}
		if weight == 0 {
			continue
		}
		randomPool.AddItem(fishSize, fishSize.Weight)
	}
	if randomPool.Size() == 0 {
		return nil, nil, neterror.ParamsInvalidError("no FishSize %d", groundId)
	}
	fishSizeConf := randomPool.RandomOne().(*jsondata.FishFriendFishSize)
	randomPool.Clear()

	// step3 哪一条鱼
	for _, fish := range fishSizeConf.FishList {
		randomPool.AddItem(fish, fish.Weight)
	}
	if randomPool.Size() == 0 {
		return nil, nil, neterror.ParamsInvalidError("no FishList %d", groundId, fishSizeConf.Type)
	}
	fishQConf := randomPool.RandomOne().(*jsondata.FishFriendFishList)
	randomPool.Clear()
	if len(fishQConf.QualityWeight) == 0 {
		return nil, nil, neterror.ParamsInvalidError("no fish QualityWeight %d", groundId, fishSizeConf.Type)
	}

	// step4 鱼的精品良品优品
	for _, qualityWeight := range fishQConf.QualityWeight {
		weight := qualityWeight.Weight
		quality := qualityWeight.Weight

		// 鱼钩加成
		qWeight, ok := fishHookConf.QWeight[quality]
		if ok {
			if qWeight.Opt == 0 {
				weight += qWeight.AddRate
			} else if qWeight.Opt == 1 {
				if weight > qWeight.AddRate {
					weight = 0
				} else {
					weight += qWeight.AddRate
				}
			}
		}

		// 鱼饵加成
		if baitConf != nil {
			qWeight, ok = baitConf.QWeight[quality]
			if ok {
				if qWeight.Opt == 0 {
					weight += qWeight.AddRate
				} else if qWeight.Opt == 1 {
					if weight > qWeight.AddRate {
						weight = 0
					} else {
						weight += qWeight.AddRate
					}
				}
			}
		}
		if weight == 0 {
			continue
		}
		randomPool.AddItem(qualityWeight, qualityWeight.Weight)
	}
	if randomPool.Size() == 0 {
		randomPool.AddItem(fishQConf.QualityWeight[0], fishQConf.QualityWeight[0].Weight)
	}
	qualityWeight := randomPool.RandomOne().(*jsondata.FishFriendQualityWeight)
	randomPool.Clear()

	// step5 鱼的重量 积分
	var heavyRange uint32 = 8000
	if len(qualityWeight.HeavyRange) == 2 {
		heavyRange = random.IntervalUU(qualityWeight.HeavyRange[0], qualityWeight.HeavyRange[1])
	}
	fishConf := config.Fish[fishQConf.ItemId]
	if fishConf == nil {
		return nil, nil, neterror.ParamsInvalidError("fishConf %d not found", fishQConf.ItemId)
	}
	ret.FishItemId = fishQConf.ItemId
	ret.Gram = int64(heavyRange) * fishConf.Size / 10000
	ret.Point = ret.Gram * fishConf.Point / 10000
	for _, award := range qualityWeight.Awards {
		randomPool.AddItem(award, award.Weight)
	}
	// 上报钓鱼积分
	s.GetPlayer().TriggerEvent(custom_id.AeAddActivityHandBookExp, int(config.SizeReportYYId), uint32(ret.Gram))
	reward := randomPool.RandomOne().(*jsondata.StdReward)
	if fishSizeConf.TipMsgId > 0 {
		itemConfig := jsondata.GetItemConfig(fishConf.ItemId)
		if itemConfig != nil {
			engine.BroadcastTipMsgById(fishSizeConf.TipMsgId, s.GetPlayer().GetId(), s.GetPlayer().GetName(), fishingGroundConf.Name, fmt.Sprintf("%.1f", float64(ret.Gram)/500), itemConfig.Name)
		}
	}
	return ret, reward, nil
}

func (s *FishingFriendSys) updatePictureGuide(groundId uint32, ret *pb3.PYYFishingRet) int64 {
	guide := s.getFishingPictureGuide(groundId)
	fishItemId := ret.FishItemId
	nowSec := time_util.NowSec()
	fishingFish, ok := guide.FishMap[fishItemId]
	if !ok {
		guide.FishMap[fishItemId] = &pb3.PYYFishingFish{
			FishId: fishItemId,
			TimeAt: nowSec,
			Gram:   ret.Gram,
			Count:  1,
		}
		return ret.Gram
	}
	var newRecordGram int64 = 0
	fishingFish.Count += 1
	if fishingFish.Gram < ret.Gram {
		fishingFish.Gram = ret.Gram
		fishingFish.TimeAt = nowSec
		newRecordGram = ret.Gram
	}
	return newRecordGram
}

func (s *FishingFriendSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_8_241
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	owner := s.GetPlayer()
	data := s.getData()
	fishhookItemId := req.FishhookItemId
	baitItemId := req.BaitItemId
	groundId := req.GroundId
	count := owner.GetItemCount(fishhookItemId, -1)
	if uint32(count) < req.Times {
		return neterror.ParamsInvalidError("count %d < req.Times %d", count, req.Times)
	}
	var retList []*pb3.PYYFishingRet
	var awardVec jsondata.StdRewardVec
	var totalPoint int64
	var totalGram int64
	var newRecordMap = make(map[uint32]int64)
	var logList []*pb3.PYYFishingLog
	var kv []*pb3.KeyValue
	var consumeVec []*jsondata.Consume
	var baitItemCount int64
	if baitItemId != 0 {
		baitItemCount = owner.GetItemCount(baitItemId, -1)
	}
	for i := uint32(0); i < req.Times; i++ {
		var mkv = &pb3.KeyValue{
			Key:   fishhookItemId,
			Value: 0,
		}
		kv = append(kv, mkv)
		consumeVec = append(consumeVec, &jsondata.Consume{
			Id:    fishhookItemId,
			Count: 1,
		})
		if baitItemCount > 0 {
			consumeVec = append(consumeVec, &jsondata.Consume{
				Id:    baitItemId,
				Count: 1,
			})
			mkv.Value = baitItemId
			baitItemCount -= 1
		}
	}
	if !owner.ConsumeByConf(consumeVec, false, common.ConsumeParams{LogId: pb3.LogId_LogPYYFishingDraw}) {
		return neterror.ParamsInvalidError("not enough")
	}
	for _, v := range kv {
		fishRet, awards, err := s.drawFish(groundId, v.Key, v.Value)
		if err != nil {
			s.LogError("err:%v", err)
			return err
		}
		if fishRet.IsFailed {
			continue
		}
		retList = append(retList, fishRet)
		if awards != nil {
			awardVec = append(awardVec, awards)
		}
		data.Gram += fishRet.Gram
		totalPoint += fishRet.Point
		totalGram += fishRet.Gram
		newRecord := s.updatePictureGuide(groundId, fishRet)
		if newRecord != 0 {
			newRecordMap[fishRet.FishItemId] = newRecord
		}
		logList = append(logList, &pb3.PYYFishingLog{
			GroundId:   groundId,
			FishItemId: fishRet.FishItemId,
			Gram:       fishRet.Gram,
			ActorId:    owner.GetId(),
			Name:       owner.GetName(),
			CreatedAt:  time_util.NowSec(),
		})
	}
	s.appendLog(logList...)
	var changeSet = make(map[uint32]struct{})
	for itemId, newGram := range newRecordMap {
		for _, ret := range retList {
			if _, ok := changeSet[ret.FishItemId]; ok {
				continue
			}
			if ret.FishItemId != itemId || ret.Gram != newGram {
				continue
			}
			ret.NewRecord = true
			changeSet[itemId] = struct{}{}
			break
		}
	}
	giveAwardVec := jsondata.MergeStdReward(awardVec)
	if len(giveAwardVec) > 0 {
		engine.GiveRewards(owner, giveAwardVec, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYFishingDraw, NoTips: true})
	}
	if totalPoint > 0 {
		owner.AddMoney(moneydef.FishingFriendPoint, totalPoint, false, pb3.LogId_LogPYYFishingDraw)
	}
	var rest = &pb3.S2C_8_241{
		ActiveId: s.Id,
		Gram:     data.Gram,
		RetList:  retList,
		Awards:   jsondata.StdRewardVecToPb3RewardVec(awardVec),
		IsShow:   !data.Auto,
	}
	s.SendProto3(8, 241, rest)
	s.SendProto3(8, 242, &pb3.S2C_8_242{
		ActiveId: s.GetId(),
		Guide:    s.getFishingPictureGuide(groundId),
	})
	event.TriggerSysEvent(custom_id.SeCrossCommonRankUpdate, &pb3.YYCrossRankParams{
		RankType: ranktype.CommonRankTypeByFishFriend,
		ActorId:  owner.GetId(),
		Score:    totalGram,
	})
	return nil
}

func (s *FishingFriendSys) appendLog(logs ...*pb3.PYYFishingLog) {
	config := jsondata.GetPyyFishFriendConfig(s.ConfName, s.ConfIdx)
	if config == nil {
		return
	}
	pers := config.LogSizePers
	if pers == 0 {
		pers = 50
	}
	all := config.LogSizeAll
	if all == 0 {
		all = 100
	}
	data := s.getData()
	logList := getPYYFishingLogList(s.Id)
	data.Logs = append(data.Logs, logs...)
	logList = append(logList, logs...)
	sort.Slice(data.Logs, func(i, j int) bool {
		return data.Logs[i].CreatedAt > data.Logs[j].CreatedAt
	})
	sort.Slice(logList, func(i, j int) bool {
		return logList[i].CreatedAt > logList[j].CreatedAt
	})
	setPYYFishingLogList(s.Id, logList)
	if uint32(len(data.Logs)) > pers {
		ret1 := make([]*pb3.PYYFishingLog, pers)
		copy(ret1, data.Logs)
		data.Logs = ret1
	}
	if uint32(len(logList)) > all {
		ret2 := make([]*pb3.PYYFishingLog, all)
		copy(ret2, logList)
		setPYYFishingLogList(s.Id, ret2)
	}
}

func (s *FishingFriendSys) c2sRecPictureAwards(msg *base.Message) error {
	var req pb3.C2S_8_243
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	config := jsondata.GetPyyFishFriendConfig(s.ConfName, s.ConfIdx)
	if config == nil {
		return neterror.ConfNotFoundError("config not found")
	}

	player := s.GetPlayer()
	groundId := req.GroundId
	pictureGuide := s.getFishingPictureGuide(groundId)
	if pictureGuide.RecAwards {
		return neterror.ParamsInvalidError("already rec %d awards", groundId)
	}

	guideConf, ok := config.PictureGuide[groundId]
	if !ok {
		return neterror.ConfNotFoundError("%d not found conf", groundId)
	}
	for _, itemId := range guideConf.ItemIds {
		_, ok := pictureGuide.FishMap[itemId]
		if !ok {
			return neterror.ParamsInvalidError("not active %d ", itemId)
		}
	}
	pictureGuide.RecAwards = true
	if len(guideConf.Awards) > 0 {
		engine.GiveRewards(player, guideConf.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYFishingRecPictureAwards, NoTips: true})
		s.GetPlayer().SendShowRewardsPop(guideConf.Awards)
	}
	s.SendProto3(8, 243, &pb3.S2C_8_243{
		ActiveId: s.Id,
		GroundId: groundId,
	})
	return nil
}

func (s *FishingFriendSys) c2sAuto(_ *base.Message) error {
	config := jsondata.GetPyyFishFriendConfig(s.ConfName, s.ConfIdx)
	if config == nil {
		return neterror.ConfNotFoundError("config not found")
	}
	obj := pyymgr.GetPlayerYYObj(s.GetPlayer(), config.PrivilegePyyId)
	if obj == nil {
		return neterror.ParamsInvalidError("not found %d privilege pyyId %d", config.PrivilegePyyId)
	}
	privilegeObj, ok := obj.(*ActivityHandBookSys)
	if !ok {
		return neterror.ParamsInvalidError("not convert privilege pyyId %d", config.PrivilegePyyId)
	}
	if privilegeObj.GetData().UnlockHighAwardsTimeAt == 0 {
		return neterror.ParamsInvalidError("not unlock %d privilege pyyId %d", config.PrivilegePyyId)
	}

	data := s.getData()
	data.Auto = !data.Auto
	s.SendProto3(8, 244, &pb3.S2C_8_244{
		ActiveId: s.Id,
		Auto:     data.Auto,
	})
	return nil
}

func (s *FishingFriendSys) c2sLogs(msg *base.Message) error {
	var req pb3.C2S_8_245
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	var rest = &pb3.S2C_8_245{
		ActiveId: s.Id,
		Global:   req.Global,
	}
	if req.Global {
		rest.Logs = getPYYFishingLogList(s.Id)
	} else {
		rest.Logs = s.getData().Logs
	}
	s.SendProto3(8, 245, rest)
	return nil
}

func getPYYFishingLogList(pyyId uint32) []*pb3.PYYFishingLog {
	staticVar := gshare.GetStaticVar()
	if staticVar == nil {
		return nil
	}
	if staticVar.FishingLogMap == nil {
		staticVar.FishingLogMap = make(map[uint32]*pb3.PYYFishingLogList)
	}
	if staticVar.FishingLogMap[pyyId] == nil {
		staticVar.FishingLogMap[pyyId] = &pb3.PYYFishingLogList{}
	}
	logList := staticVar.FishingLogMap[pyyId]
	return logList.Logs
}

func setPYYFishingLogList(pyyId uint32, logList []*pb3.PYYFishingLog) {
	staticVar := gshare.GetStaticVar()
	if staticVar == nil {
		return
	}
	if staticVar.FishingLogMap == nil {
		staticVar.FishingLogMap = make(map[uint32]*pb3.PYYFishingLogList)
	}
	if staticVar.FishingLogMap[pyyId] == nil {
		staticVar.FishingLogMap[pyyId] = &pb3.PYYFishingLogList{}
	}
	staticVar.FishingLogMap[pyyId].Logs = logList
	return
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYFishingFriend, func() iface.IPlayerYY {
		return &FishingFriendSys{}
	})
	net.RegisterYYSysProtoV2(8, 241, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*FishingFriendSys).c2sDraw
	})
	net.RegisterYYSysProtoV2(8, 243, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*FishingFriendSys).c2sRecPictureAwards
	})
	net.RegisterYYSysProtoV2(8, 244, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*FishingFriendSys).c2sAuto
	})
	net.RegisterYYSysProtoV2(8, 245, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*FishingFriendSys).c2sLogs
	})
	gmevent.Register("FishingFriendSys.setAuto", func(player iface.IPlayer, args ...string) bool {
		pyymgr.EachPlayerAllYYObj(player, yydefine.PYYFishingFriend, func(obj iface.IPlayerYY) {
			obj.(*FishingFriendSys).getData().Auto = !obj.(*FishingFriendSys).getData().Auto
			obj.(*FishingFriendSys).s2cInfo()
			return
		})
		return true
	}, 1)
}
