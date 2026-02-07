/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 炼丹
**/

package actorsystem

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"sort"
	"time"

	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils/pie"
)

const (
	NumTypeInteger = 1
	NumTypePercent = 2

	alchemyFail = 1 // 炼丹失败的品质
)

type AlchemySys struct {
	Base
	AlchemyChances []*pb3.AlchemyChance
}

func (s *AlchemySys) OnLogin() {
	s.reStatChance()
	s.S2CInfo()
}

func (s *AlchemySys) OnOpen() {
	s.GetData().IsFirst = true
	s.reStatChance()
	s.checkOpenActive()
	s.S2CInfo()
}

func (s *AlchemySys) OnReconnect() {
	s.reStatChance()
	s.S2CInfo()
}

func (s *AlchemySys) checkOpenActive() {
	conf, ok := jsondata.GetAlchemyConf()
	if !ok {
		return
	}
	for _, stageConf := range conf.StageConf {
		for _, prescriptionConf := range stageConf.PrescriptionConf {
			if !prescriptionConf.Activation {
				continue
			}

			err := s.active(prescriptionConf.ItemId, pb3.LogId_LogActiveAlchemyPrescription, true)
			if err != nil {
				s.LogDebug("err:%v", err)
			}
		}
	}
}

func (s *AlchemySys) GetData() *pb3.AlchemyState {
	if s.GetBinaryData().AlchemyState == nil {
		s.GetBinaryData().AlchemyState = &pb3.AlchemyState{}
	}
	state := s.GetBinaryData().AlchemyState
	if state.StagePrescriptionMap == nil {
		state.StagePrescriptionMap = make(map[uint32]*pb3.AlchemPrescriptions)
	}
	if state.Lv == 0 {
		state.Lv = 1
	}
	return state
}

func (s *AlchemySys) S2CInfo() {
	s.SendProto3(155, 1, &pb3.S2C_155_1{
		State: s.GetData(),
	})
}

// 重新统计加成
func (s *AlchemySys) reStatChance() {
	data := s.GetData()
	lv := data.GetLv()
	var m = make(map[string]*pb3.AlchemyChance)

	// 等级覆盖加成
	conf, ok := jsondata.GetAlchemyLevelConf(lv)
	if ok {
		for _, chanceConf := range conf.Chance {
			k := fmt.Sprintf("%d_%d_%d_%d", chanceConf.Stage, chanceConf.PrescriptionType, chanceConf.QualityType, chanceConf.NumType)
			chance, ok := m[k]
			if !ok {
				m[k] = &pb3.AlchemyChance{
					Stage:            chanceConf.Stage,
					PrescriptionType: chanceConf.PrescriptionType,
					QualityType:      chanceConf.QualityType,
					Weight:           chanceConf.Weight,
					SubCd:            chanceConf.SubCd,
					NumType:          chanceConf.NumType,
				}
				continue
			}
			chance.Weight += chanceConf.Weight
			chance.SubCd += chanceConf.SubCd
			m[k] = chance
		}
	}

	// 遍历月卡
	if len(data.MonthCardChange) > 0 {
		for _, chanceConf := range data.MonthCardChange {
			k := fmt.Sprintf("%d_%d_%d_%d", chanceConf.Stage, chanceConf.PrescriptionType, chanceConf.QualityType, chanceConf.NumType)
			chance, ok := m[k]
			if !ok {
				m[k] = &pb3.AlchemyChance{
					Stage:            chanceConf.Stage,
					PrescriptionType: chanceConf.PrescriptionType,
					QualityType:      chanceConf.QualityType,
					Weight:           chanceConf.Weight,
					SubCd:            chanceConf.SubCd,
					NumType:          chanceConf.NumType,
				}
				continue
			}
			chance.Weight += chanceConf.Weight
			chance.SubCd += chanceConf.SubCd
		}
	}

	s.AlchemyChances = nil
	for k := range m {
		s.AlchemyChances = append(s.AlchemyChances, m[k])
	}
}

func (s *AlchemySys) checkActive(stage uint32, prescriptionId uint32) bool {
	data := s.GetData()
	stagePrescriptionMap := data.StagePrescriptionMap
	prescription, ok := stagePrescriptionMap[stage]
	if !ok || !pie.Uint32s(prescription.PrescriptionIds).Contains(prescriptionId) {
		return false
	}
	return true
}

func (s *AlchemySys) addExp(exp uint32) {
	if exp <= 0 {
		return
	}
	var logSt = &pb3.LogExpLv{}
	data := s.GetData()
	logSt.FromLevel = data.Lv
	logSt.Exp = int64(exp)
	var curLv, curExp = data.Lv, data.CurExp + exp
	alchemyConf, ok := jsondata.GetAlchemyConf()
	if !ok {
		return
	}
	// 避免死循环
	for i := 0; i < len(alchemyConf.LevelConf); i++ {
		conf, ok := jsondata.GetAlchemyLevelConf(curLv)
		if !ok {
			break
		}
		// 没有下一级了
		_, ok = jsondata.GetAlchemyLevelConf(curLv + 1)
		if !ok {
			break
		}
		if curExp < conf.Exp {
			break
		}
		curExp -= conf.Exp
		curLv += 1
	}
	data.Lv, data.CurExp = curLv, curExp
	s.reStatChance()
	s.SendProto3(155, 3, &pb3.S2C_155_3{
		CurExp: curExp,
		Lv:     curLv,
	})
	logSt.ToLevel = data.Lv
	logworker.LogExpLv(s.GetOwner(), pb3.LogId_LogAlchemyAddExp, logSt)
}

// 获取减cd的加成
func (s *AlchemySys) getSubCdChance(stage, prescriptionType, QualityType, numType uint32) uint32 {
	var subCd uint32
	for _, chance := range s.AlchemyChances {
		if chance.NumType != numType {
			continue
		}

		// 全局减cd
		if chance.Stage == 0 &&
			chance.PrescriptionType == 0 &&
			chance.QualityType == 0 &&
			chance.SubCd > 0 {
			subCd += chance.SubCd
			continue
		}

		if chance.Stage == stage &&
			chance.PrescriptionType == 0 &&
			chance.QualityType == 0 &&
			chance.SubCd > 0 {
			subCd += chance.SubCd
			continue
		}

		if chance.Stage == stage &&
			chance.PrescriptionType == prescriptionType &&
			chance.QualityType == 0 &&
			chance.SubCd > 0 {
			subCd += chance.SubCd
			continue
		}

		if chance.Stage == stage &&
			chance.PrescriptionType == prescriptionType &&
			chance.QualityType == QualityType &&
			chance.SubCd > 0 {
			subCd += chance.SubCd
			continue
		}
	}
	return subCd
}

// 获取首次炼丹配制
func (s *AlchemySys) getFirstStartAlchemyConf() (state uint32, prescriptionId uint32, count uint32, qualityType uint32, err error) {
	conf, ok := jsondata.GetAlchemyConf()
	if !ok {
		err = neterror.ConfNotFoundError("not found alchemy conf")
		s.GetOwner().LogWarn("err:%v", err)
		return
	}

	if len(conf.First) < 4 {
		err = neterror.ConfNotFoundError("not found first alchemy conf")
		s.GetOwner().LogWarn("err:%v", err)
		return
	}

	ok = true
	state, prescriptionId, count, qualityType = conf.First[0], conf.First[1], conf.First[2], conf.First[3]
	return
}

// 开始炼丹
func (s *AlchemySys) startAlchemy(reqStage, reqCount, reqPrescriptionId, reqFurnaceType, qualityType, confCount uint32) (*pb3.S2C_155_2, error) {
	data := s.GetData()
	owner := s.GetOwner()

	// 丹方状态
	if !s.checkActive(reqStage, reqPrescriptionId) {
		return nil, neterror.ParamsInvalidError("un active stagePrescription , state is %d, id is %d", reqStage, reqPrescriptionId)
	}

	// 丹方
	prescriptionConf, ok := jsondata.GetAlchemyPrescriptionConf(reqStage, reqPrescriptionId)
	if !ok {
		return nil, neterror.ConfNotFoundError("not found prescriptionConf, state is %d, id is %d", reqStage, reqPrescriptionId)
	}

	// 丹炉
	conf, ok := jsondata.GetAlchemyFurnaceConf(reqStage, reqPrescriptionId, reqFurnaceType)
	if !ok {
		return nil, neterror.ConfNotFoundError("not found stagePrescriptionConf, state is %d, id is %d,typ is %d", reqStage, reqPrescriptionId, reqFurnaceType)
	}

	alchemyConf, ok := jsondata.GetAlchemyConf()
	if !ok {
		return nil, neterror.ParamsInvalidError("alchemyConf not found")
	}

	// 提前消耗
	if len(conf.Consume) > 0 {
		var cs jsondata.ConsumeVec
		for i := uint32(0); i < reqCount; i++ {
			cs = append(cs, conf.Consume...)
		}
		if !owner.ConsumeByConf(cs, false, common.ConsumeParams{LogId: pb3.LogId_LogConsumeByAlchemy}) {
			err := neterror.ConsumeFailedError("Consume Failed")
			s.GetOwner().LogWarn("err:%v", err)
			owner.SendTipMsg(tipmsgid.TpUseItemFailed)
			return nil, err
		}
	}

	// 指定炼成的品质
	var elixirConf *jsondata.AlchemyElixirConf
	if qualityType > 0 {
		for i := range conf.ElixirConf {
			if conf.ElixirConf[i].Quality.Type != qualityType {
				continue
			}
			elixirConf = conf.ElixirConf[i]
			break
		}
	}

	// 快速炼丹特权
	var privilegeTotal int64
	if alchemyConf.AlchemyQuickActorLv < owner.GetLevel() {
		privilegeTotal = 1
	}

	// 炼丹
	var ing = &pb3.AlchemyIng{
		Stage:          reqStage,
		PrescriptionId: reqPrescriptionId,
		FurnaceType:    reqFurnaceType,
	}
	var list []*pb3.AlchemyIngEntry
	endTimer := time.Now()
	for i := uint32(0); i < reqCount; i++ {
		var costTime uint32
		// 指定炼成的品质
		if qualityType > 0 && confCount > 0 && elixirConf != nil {
			costTime = conf.Time
			if privilegeTotal > 0 {
				costTime = 0
			}

			endTimer = endTimer.Add(time.Duration(costTime) * time.Second)
			list = append(list, &pb3.AlchemyIngEntry{
				QualityType: qualityType,
				EndTime:     uint32(endTimer.Unix()),
			})
			confCount--
			continue
		}

		// 计算最终的权重
		elixirList := s.calcChance(reqStage, prescriptionConf.Type, conf.ElixirConf)
		pool := new(random.Pool)
		for i := range elixirList {
			pool.AddItem(elixirList[i], elixirList[i].Weight)
		}
		elixirConf := pool.RandomOne().(*jsondata.AlchemyElixirConf)

		costTime = s.calcSubSd(reqStage, prescriptionConf.Type, elixirConf.Quality.Type, conf.Time)
		if privilegeTotal > 0 {
			costTime = 0
		}
		endTimer = endTimer.Add(time.Duration(costTime) * time.Second)
		list = append(list, &pb3.AlchemyIngEntry{
			QualityType: elixirConf.Quality.Type,
			EndTime:     uint32(endTimer.Unix()),
		})
	}

	if len(list) == 0 {
		err := neterror.ParamsInvalidError("not enough list")
		owner.LogWarn("err:%v", err)
		return nil, err
	}

	// 记录数据
	ing.List = list
	data.AlchemyIng = ing
	data.LastEndTime = uint32(endTimer.Unix())

	// 触发丹方任务
	s.triggerPrescriptionQuestEvent(prescriptionConf.ItemId, reqCount)

	// 快速炼丹特权
	if privilegeTotal > 0 {
		data.LastEndTime = data.LastEndTime + 1
	}

	return &pb3.S2C_155_2{
		AlchemyIng:  data.AlchemyIng,
		LastEndTime: data.LastEndTime,
		StartTime:   time_util.NowSec(),
	}, nil
}

// 触发丹方任务
func (s *AlchemySys) triggerPrescriptionQuestEvent(itemId uint32, count uint32) {
	owner := s.GetOwner()
	owner.TriggerQuestEvent(custom_id.QttAnyAlchemyTimes, 0, int64(count))
	owner.TriggerQuestEvent(custom_id.QttSpecificAlchemyTimes, itemId, int64(count))
	owner.TriggerQuestEvent(custom_id.QttAccumulateAnyAlchemyTimes, 0, int64(count))
	owner.TriggerQuestEvent(custom_id.QttAccumulateSpecificAlchemyTimes, itemId, int64(count))
}

// 获取完成、未开始、失败的炼丹列表
func (s *AlchemySys) getCompleteAndUnStartAndFailList() ([]*pb3.AlchemyIngEntry, []*pb3.AlchemyIngEntry, []*pb3.AlchemyIngEntry) {
	ing := s.GetData().AlchemyIng
	list := ing.List
	now := time_util.NowSec()
	var completeList, unStartList, failList []*pb3.AlchemyIngEntry

	sort.Slice(list, func(i, j int) bool {
		return list[i].EndTime < list[j].EndTime
	})

	for i := range list {
		alchemyIng := list[i]
		if alchemyIng.EndTime <= now {
			completeList = append(completeList, alchemyIng)
			continue
		}
		unStartList = append(unStartList, alchemyIng)
	}

	if len(unStartList) != 0 && len(completeList) != 0 {
		if completeList[len(completeList)-1].EndTime < now && now < unStartList[0].EndTime {
			failList = append(failList, unStartList[0])
			if len(unStartList) == 1 {
				unStartList = nil
			} else if len(unStartList) >= 2 {
				unStartList = unStartList[1:]
			}
		}
	}

	return completeList, unStartList, failList
}

// 获取成功、失败的数量
func (s *AlchemySys) getSuccessAndFailNum(list ...*pb3.AlchemyIngEntry) (uint32, uint32) {
	var success, fail uint32
	for _, ing := range list {
		if ing.QualityType == alchemyFail {
			fail++
			continue
		}
		success++
	}
	return success, fail
}

// 计算退还的材料
func (s *AlchemySys) calcMaterialsByUnStartList(ing *pb3.AlchemyIng, list ...*pb3.AlchemyIngEntry) (jsondata.StdRewardVec, error) {
	var vec jsondata.StdRewardVec

	if len(list) == 0 {
		return vec, nil
	}

	conf, ok := jsondata.GetAlchemyFurnaceConf(ing.Stage, ing.PrescriptionId, ing.FurnaceType)
	if !ok {
		err := neterror.ConfNotFoundError("alchemy prescription conf not found , state %d , prescription id is %d , Furnace type is %d", ing.Stage, ing.PrescriptionId, ing.FurnaceType)
		s.GetOwner().LogWarn("err:%v", err)
		return nil, err
	}

	for _, award := range conf.ReturnAwards {
		vec = append(vec, &jsondata.StdReward{
			Id:    award.Id,
			Count: award.Count * int64(len(list)),
			Bind:  award.Bind,
			Job:   award.Job,
		})
	}

	return vec, nil
}

// 计算奖励
func (s *AlchemySys) calcRewards(ing *pb3.AlchemyIng, alchemyIngs ...*pb3.AlchemyIngEntry) (jsondata.StdRewardVec, uint32, error) {
	var rewards jsondata.StdRewardVec
	var totalExp uint32

	if len(alchemyIngs) == 0 {
		return rewards, totalExp, nil
	}

	conf, ok := jsondata.GetAlchemyFurnaceConf(ing.Stage, ing.PrescriptionId, ing.FurnaceType)
	if !ok {
		err := neterror.ConfNotFoundError("not found stagePrescriptionConf, state is %d, id is %d, furnaceType %d ", ing.Stage, ing.PrescriptionId, ing.FurnaceType)
		s.GetOwner().LogWarn("err:%v", err)
		return nil, 0, err
	}

	for i := range alchemyIngs {
		alchemyIng := alchemyIngs[i]
		var recElixirConf *jsondata.AlchemyElixirConf
		for idx := range conf.ElixirConf {
			elixirConf := conf.ElixirConf[idx]
			if elixirConf.Quality.Type != alchemyIng.QualityType {
				continue
			}
			recElixirConf = elixirConf
			break
		}

		if recElixirConf == nil {
			continue
		}

		rewards = append(rewards, recElixirConf.Awards...)
		totalExp += conf.Exp * recElixirConf.Quality.Num
	}

	return rewards, totalExp, nil
}

// 获取加成
func (s *AlchemySys) getWeightChanceMapKeyByQualityType(stage, prescriptionType, numType uint32) map[uint32]*pb3.AlchemyChance {
	var chanceMap = make(map[uint32]*pb3.AlchemyChance)
	for _, chance := range s.AlchemyChances {
		if chance.NumType != numType {
			continue
		}

		// 阶 丹类型
		if chance.Stage != stage || chance.PrescriptionType != prescriptionType {
			continue
		}
		chanceMap[chance.QualityType] = proto.Clone(chance).(*pb3.AlchemyChance)
	}
	return chanceMap
}

// 计算加成
func (s *AlchemySys) calcChance(stage uint32, prescriptionType uint32, elixirList jsondata.AlchemyElixirConfVec) jsondata.AlchemyElixirConfVec {
	// 所有数值加成 key by QualityType
	chanceMap := s.getWeightChanceMapKeyByQualityType(stage, prescriptionType, NumTypeInteger)

	// 所有百分比加成 key by QualityType
	chanceMapV2 := s.getWeightChanceMapKeyByQualityType(stage, prescriptionType, NumTypePercent)

	// 没有加成
	if chanceMap == nil && chanceMapV2 == nil {
		return elixirList.Copy()
	}

	// 计算加成
	var addList = elixirList.Copy()
	sort.Slice(addList, func(i, j int) bool {
		return addList[i].Quality.Type > addList[j].Quality.Type // 大到小
	})

	// 只有一个 那就没有补齐的意义了
	if len(addList) == 1 {
		return addList
	}

	for addIdx := range addList {
		elixir := addList[addIdx]
		if elixir.Weight == 0 {
			continue
		}

		// 计算一下加成率
		var needAdd uint32
		var needSub uint32

		chance, ok1 := chanceMap[elixir.Quality.Type]
		if ok1 {
			needAdd += chance.Weight
			needSub += chance.Weight
		}

		chanceV2, ok2 := chanceMapV2[elixir.Quality.Type]
		if ok2 {
			needAdd += (chanceV2.Weight * elixir.Weight) / 100
			needSub += (chanceV2.Weight * elixir.Weight) / 100
		}

		if !ok1 && !ok2 {
			continue
		}

		for subIdx := len(addList) - 1; subIdx > addIdx; subIdx-- {
			if needSub < addList[subIdx].Weight {
				addList[subIdx].Weight -= needSub
				needSub = 0
				break
			} else {
				needSub -= addList[subIdx].Weight
				addList[subIdx].Weight = 0
			}
		}

		// 20 30 20 20 10
		// 概率补齐成功
		if needSub == 0 {
			elixir.Weight += needAdd
		} else {
			elixir.Weight += needAdd - needSub // 出现它 表示已经没有来减了 那么就应该结束了
			for subIdx := len(addList) - 1; subIdx > addIdx; subIdx-- {
				addList[subIdx].Weight = 0
			}
			break
		}
	}

	return addList
}

// 增加月卡加成
func (s *AlchemySys) addMonthChance() {
	owner := s.GetOwner()
	total, _ := owner.GetPrivilege(privilegedef.EnumHeightFurnace)
	if total == 0 {
		return
	}

	data := s.GetData()
	conf, ok := jsondata.GetAlchemyConf()
	if !ok {
		return
	}

	for i := range conf.MonthCardChance {
		chanceConf := conf.MonthCardChance[i]
		data.MonthCardChange = append(data.MonthCardChange, &pb3.AlchemyChance{
			Stage:            chanceConf.Stage,
			PrescriptionType: chanceConf.PrescriptionType,
			QualityType:      chanceConf.QualityType,
			Weight:           chanceConf.Weight,
			SubCd:            chanceConf.SubCd,
			NumType:          chanceConf.NumType,
		})
	}

	s.reStatChance()
	s.SendProto3(155, 4, &pb3.S2C_155_4{MonthCardChange: data.MonthCardChange})
}

// 移除月卡加成
func (s *AlchemySys) subMonthChance() {
	s.GetData().MonthCardChange = nil
	s.reStatChance()
	s.SendProto3(155, 7, &pb3.S2C_155_7{})
}

// 计算减cd
func (s *AlchemySys) calcSubSd(stage, prescriptionType, qualityType uint32, cd uint32) uint32 {
	subCd := s.getSubCdChance(stage, prescriptionType, qualityType, NumTypeInteger)
	subCdPercent := s.getSubCdChance(stage, prescriptionType, qualityType, NumTypePercent)
	if subCd > cd {
		return 0
	}
	cd = cd - subCd

	if cd == 0 {
		return 0
	}

	var needSub = (cd * subCdPercent) / 100
	if needSub > cd {
		return 0
	}
	return cd - needSub
}

// 激活丹方
func (s *AlchemySys) active(prescriptionItemId uint32, logId pb3.LogId, skipTip bool) error {
	owner := s.GetOwner()
	owner.LogInfo("active prescriptionItemId %d", prescriptionItemId)
	alchemyConf, ok := jsondata.GetAlchemyConf()
	if !ok {
		return neterror.ConfNotFoundError("not found alchemy")
	}
	var stage, prescriptionId uint32
	for k := range alchemyConf.StageConf {
		stageConf := alchemyConf.StageConf[k]
		for _, conf := range stageConf.PrescriptionConf {
			if conf.ItemId == prescriptionItemId {
				stage = stageConf.Stage
				prescriptionId = conf.Id
				break
			}
		}
	}

	if stage == 0 && prescriptionId == 0 {
		return neterror.ParamsInvalidError("alchemy prescription not found")
	}

	if s.checkActive(stage, prescriptionId) {
		return neterror.ParamsInvalidError("already active , stage %v prescription %v", stage, prescriptionId)
	}

	pConf, ok := jsondata.GetAlchemyPrescriptionConf(stage, prescriptionId)
	if !ok {
		return neterror.ParamsInvalidError("not found conf, stage %v prescription %v", stage, prescriptionId)
	}

	data := s.GetData()
	prescriptions, ok := data.StagePrescriptionMap[stage]
	if !ok {
		prescriptions = &pb3.AlchemPrescriptions{}
	}

	if pie.Uint32s(prescriptions.PrescriptionIds).Contains(prescriptionId) {
		return neterror.ParamsInvalidError("already active id %d , itemId is %d", prescriptionId, prescriptionItemId)
	}

	prescriptions.PrescriptionIds = pie.Uint32s(prescriptions.PrescriptionIds).Append(prescriptionId).Unique()
	data.StagePrescriptionMap[stage] = prescriptions
	s.SendProto3(155, 5, &pb3.S2C_155_5{
		Stage:          stage,
		PrescriptionId: prescriptionId,
	})

	if itemConf := jsondata.GetItemConfig(pConf.ItemId); itemConf != nil && !skipTip {
		owner.SendTipMsg(tipmsgid.AlchemyActivePrescription, itemConf.Name)
	}

	logworker.LogPlayerBehavior(owner, logId, &pb3.LogPlayerCounter{
		NumArgs: uint64(prescriptionItemId),
	})

	return nil
}

// 开始炼丹
func (s *AlchemySys) c2sStartAlchemy(msg *base.Message) error {
	data := s.GetData()
	owner := s.GetOwner()
	if data.LastEndTime > 0 {
		s.GetOwner().LogWarn("un end last alchemy , last end time is %d", data.LastEndTime)
		owner.SendTipMsg(tipmsgid.TPRevTimeNotReach)
		return nil
	}

	var req pb3.C2S_155_1
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.Wrap(err)
	}

	reqStage, reqCount, reqPrescriptionId, reqFurnaceType := req.Stage, req.Count, req.PrescriptionId, req.FurnaceType

	var confCount, qualityType uint32
	if data.IsFirst {
		_, confCount, _, qualityType, err = s.getFirstStartAlchemyConf()
		if err != nil {
			return neterror.Wrap(err)
		}
	}

	if reqCount > 100 {
		return neterror.ParamsInvalidError("count to long, req count %d", reqCount)
	}

	rsp, err := s.startAlchemy(reqStage, reqCount, reqPrescriptionId, reqFurnaceType, qualityType, confCount)
	if err != nil {
		return neterror.Wrap(err)
	}

	logArg := logworker.ConvertJsonStr(map[string]interface{}{
		"firstStartAlchemy": data.IsFirst,
		"reqStage":          reqStage,
		"reqCount":          reqCount,
		"reqPrescriptionId": reqPrescriptionId,
		"reqFurnaceType":    reqFurnaceType,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogStartAlchemy, &pb3.LogPlayerCounter{
		StrArgs: logArg,
	})

	if data.IsFirst {
		data.IsFirst = false
	}
	s.SendProto3(155, 2, rsp)
	return nil
}

// 领奖
func (s *AlchemySys) c2sRewards(msg *base.Message) error {
	var req pb3.C2S_155_2
	owner := s.GetOwner()
	data := s.GetData()
	if err := msg.UnPackPb3Msg(&req); nil != err {
		owner.LogError("err:%v", err)
		return err
	}

	if data.AlchemyIng == nil || len(data.AlchemyIng.List) == 0 {
		s.GetOwner().LogTrace("alchemy ing list is empty")
		return nil
	}

	// 获取已完成的和未开始的
	var completeList, unStartList, failList []*pb3.AlchemyIngEntry
	total, _ := owner.GetPrivilege(privilegedef.EnumAlchemyQuickReceive)
	if total > 0 {
		completeList = data.AlchemyIng.List
	} else {
		completeList, unStartList, failList = s.getCompleteAndUnStartAndFailList()
	}

	rewards, totalExp, err := s.calcRewards(data.AlchemyIng, completeList...)
	if err != nil {
		return neterror.Wrap(err)
	}

	s.addExp(totalExp)
	data.LastEndTime = 0

	var stdRewards []*pb3.StdAward
	if len(rewards) > 0 {
		stdRewards = jsondata.Pb3RewardMergeStdReward(stdRewards, rewards)
		engine.GiveRewards(owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogAwardByAlchemy})
		s.triggerRetQuestEvent(rewards)
	}

	success, fail := s.getSuccessAndFailNum(completeList...)
	var rsp = &pb3.S2C_155_6{
		RecList: stdRewards,
		Success: success,
		Fail:    fail + uint32(len(failList)),
	}

	if req.IsStop {
		returnAwards, err := s.calcMaterialsByUnStartList(data.AlchemyIng, unStartList...)
		if err != nil {
			s.GetOwner().LogError("err:%v", err)
		}
		if len(returnAwards) > 0 {
			engine.GiveRewards(owner, returnAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogAwardByAlchemyReturnMaterials})
		}
		rsp.IsStop = req.IsStop
		rsp.ReturnAwards = jsondata.StdRewardVecToPb3RewardVec(returnAwards)
	}

	s.broTips(data.AlchemyIng, completeList)

	data.AlchemyIng = nil
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogRecAlchemyResult, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"success": rsp.Success,
			"fail":    rsp.Fail,
			"isStop":  rsp.IsStop,
		}),
	})
	s.SendProto3(155, 6, rsp)

	return nil
}

func (s *AlchemySys) broTips(ing *pb3.AlchemyIng, list []*pb3.AlchemyIngEntry) {
	if len(list) == 0 {
		return
	}

	var broSet = make(map[uint32]string)
	conf, ok := jsondata.GetAlchemyFurnaceConf(ing.Stage, ing.PrescriptionId, ing.FurnaceType)
	if !ok {
		return
	}

	for _, elixirConf := range conf.ElixirConf {
		if elixirConf.TipsId == 0 {
			continue
		}
		broSet[elixirConf.Quality.Type] = elixirConf.Name
	}

	var broList = pie.Uint32s([]uint32{})
	for _, alchemyIng := range list {
		_, ok := broSet[alchemyIng.QualityType]
		if !ok {
			continue
		}
		broList = broList.Append(alchemyIng.QualityType)
	}
	broList = broList.Sort().Reverse().Unique()
	broList.Each(func(u uint32) {
		engine.BroadcastTipMsgById(tipmsgid.ElixirSuccessTips, s.GetOwner().GetName(), ing.Stage, broSet[u])
	})
}

// 首次领取赞助豪礼 把当前正在进行的炼丹全都给完成
func (s *AlchemySys) changeTimeQuickly() {
	data := s.GetData()
	if data.AlchemyIng == nil {
		return
	}
	nowSec := time_util.NowSec()
	for _, entry := range data.AlchemyIng.List {
		if entry.EndTime <= nowSec {
			continue
		}
		entry.EndTime = nowSec
	}
	data.LastEndTime = nowSec

	s.SendProto3(155, 2, &pb3.S2C_155_2{
		AlchemyIng:  data.AlchemyIng,
		LastEndTime: data.LastEndTime,
		StartTime:   nowSec,
	})
}

func (s *AlchemySys) triggerRetQuestEvent(list jsondata.StdRewardVec) {
	var qretMap = make(map[uint32]int64)
	for _, entry := range list {
		qretMap[entry.Id]++
	}
	for itemId, count := range qretMap {
		s.GetOwner().TriggerQuestEvent(custom_id.QttGetAlchemyItems, itemId, count)

		itemConf := jsondata.GetItemConfig(itemId)
		if itemConf != nil {
			s.GetOwner().TriggerEvent(custom_id.AeFaBaoTalentEvent, &custom_id.FaBaoTalentEvent{
				Cond:   custom_id.FaBaoTalentCondAlchemyItem,
				Param0: itemConf.Quality,
				Count:  uint32(count),
			})
		}
	}
}

// 使用丹方道具
func useItemAlchemyPrescription(player iface.IPlayer, param *miscitem.UseItemParamSt, _ *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	s, ok := player.GetSysObj(sysdef.SiAlchemy).(*AlchemySys)
	if !ok {
		return
	}
	err := s.active(param.ItemId, pb3.LogId_LogActiveAlchemyPrescription, false)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return
	}
	return true, true, param.Count
}

func handleAePrivilegeCardActivated(player iface.IPlayer, args ...interface{}) {
	if len(args) != 1 {
		return
	}
	val, ok := args[0].(argsdef.AePvCardActivatedArg)
	if !ok {
		player.LogDebug("argsdef.AePvCardActivatedArg is %v", args[0])
		return
	}
	if val != privilegedef.PrivilegeCardType_Month {
		player.LogWarn("type not equal , val is %d", val)
		return
	}
	s := player.GetSysObj(sysdef.SiAlchemy).(*AlchemySys)
	s.addMonthChance()
}

func handleAePrivilegeCardDisActivated(player iface.IPlayer, args ...interface{}) {
	if len(args) != 1 {
		return
	}
	val, ok := args[0].(argsdef.AePvCardActivatedArg)
	if !ok {
		player.LogDebug("argsdef.AePvCardActivatedArg is %v", args[0])
		return
	}
	if val != privilegedef.PrivilegeCardType_Month {
		player.LogWarn("type not equal , val is %d", val)
		return
	}
	s := player.GetSysObj(sysdef.SiAlchemy).(*AlchemySys)
	s.subMonthChance()
}

func handleAeReceiveSponsorGift(player iface.IPlayer, args ...interface{}) {
	total, _ := player.GetPrivilege(privilegedef.EnumAlchemyQuickReceive)
	if total == 0 {
		return
	}
	s := player.GetSysObj(sysdef.SiAlchemy).(*AlchemySys)
	s.changeTimeQuickly()
}

func init() {
	RegisterSysClass(sysdef.SiAlchemy, func() iface.ISystem {
		return &AlchemySys{}
	})
	net.RegisterSysProtoV2(155, 1, sysdef.SiAlchemy, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AlchemySys).c2sStartAlchemy
	})
	net.RegisterSysProtoV2(155, 2, sysdef.SiAlchemy, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AlchemySys).c2sRewards
	})

	// 注册道具使用
	miscitem.RegCommonUseItemHandle(itemdef.UseItemAlchemyPrescription, useItemAlchemyPrescription)
	event.RegActorEvent(custom_id.AePrivilegeCardActivated, handleAePrivilegeCardActivated)
	event.RegActorEvent(custom_id.AePrivilegeCardDisActivated, handleAePrivilegeCardDisActivated)
	event.RegActorEvent(custom_id.AeReceiveSponsorGift, handleAeReceiveSponsorGift)
}
