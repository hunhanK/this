package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/reachconddef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"math"

	"github.com/gzjjyz/logger"
)

type ReachStandardMissionChecker func(player iface.IPlayer, args []int64) bool

var missionCheckers = make(map[uint32]ReachStandardMissionChecker)

func CheckReach(actor iface.IPlayer, reachType uint32, val []int64) bool {
	if checker, ok := missionCheckers[reachType]; ok {
		return checker(actor, val)
	}
	return false
}

func RegisterReachStandardMissionChecker(missionType uint32, checker ReachStandardMissionChecker) {
	_, ok := missionCheckers[missionType]
	if ok {
		logger.LogWarn("Re register ReachStandardMissionValCollector missionType %d", missionType)
	}

	missionCheckers[missionType] = checker
}

func rider_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiRider).(*RiderSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) == 0 {
		return false
	}

	return sys.GetLevel() >= uint32(args[0])
}

func weapon_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiWeapon).(*WeaponSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) == 0 {
		return false
	}

	return sys.GetLevel() >= uint32(args[0])
}

func fairyWing_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFairyWing).(*FairyWingSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) == 0 {
		return false
	}

	return sys.GetLevel() >= uint32(args[0])
}

func fabao_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiNewFabao).(*FaBaoSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) == 0 {
		return false
	}

	state := sys.state()
	for _, bao := range state.FaBaoMap {
		if bao.Lv >= uint32(args[0]) {
			return true
		}
	}
	return false
}

func level_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	if len(args) == 0 {
		return false
	}

	return player.GetLevel() >= uint32(args[0])
}

func gemTotalLevel_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiGem).(*GemSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) == 0 {
		return false
	}

	return sys.GetTotalGemLevel() >= uint32(args[0])
}

func dragonLevel_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) == 0 {
		return false
	}

	totalLv := GetFourSymbolsLvByType(player, []uint32{0: custom_id.FourSymbolsDragon})

	return totalLv >= uint32(args[0])
}

func tigerLevel_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) == 0 {
		return false
	}

	totalLv := GetFourSymbolsLvByType(player, []uint32{0: custom_id.FourSymbolsTiger})

	return totalLv >= uint32(args[0])
}

func rosefinch_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) == 0 {
		return false
	}

	totalLv := GetFourSymbolsLvByType(player, []uint32{0: custom_id.FourSymbolsRosefinch})

	return totalLv >= uint32(args[0])
}

func tortoise_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) == 0 {
		return false
	}

	totalLv := GetFourSymbolsLvByType(player, []uint32{0: custom_id.FourSymbolsTortoise})

	return totalLv >= uint32(args[0])
}

func strongLv_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiStrong).(*EquipStrongSystem)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) == 0 {
		return false
	}

	return sys.GetEquipStrongAllLv() >= uint32(args[0])
}

func godRider_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiRider).(*RiderSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 2 {
		return false
	}
	nedCount := uint32(args[0])
	nedStage := uint32(args[1])
	count := sys.principalSuit.GetCountMoreThanTheStage(nedStage)
	return count >= nedCount
}

func godWing_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFairyWing).(*FairyWingSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 2 {
		return false
	}
	nedCount := uint32(args[0])
	nedStage := uint32(args[1])
	count := sys.principalSuit.GetCountMoreThanTheStage(nedStage)
	return count >= nedCount
}

func godDragon_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 2 {
		return false
	}
	principalSuit, ok := sys.principalSuit[custom_id.FourSymbolsDragon]
	if !ok {
		return false
	}
	nedCount := uint32(args[0])
	nedStage := uint32(args[1])
	count := principalSuit.GetCountMoreThanTheStage(nedStage)
	return count >= nedCount
}

func godTiger_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 2 {
		return false
	}
	principalSuit, ok := sys.principalSuit[custom_id.FourSymbolsTiger]
	if !ok {
		return false
	}
	nedCount := uint32(args[0])
	nedStage := uint32(args[1])
	count := principalSuit.GetCountMoreThanTheStage(nedStage)
	return count >= nedCount
}

func godRosefinch_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 2 {
		return false
	}
	principalSuit, ok := sys.principalSuit[custom_id.FourSymbolsRosefinch]
	if !ok {
		return false
	}
	nedCount := uint32(args[0])
	nedStage := uint32(args[1])
	count := principalSuit.GetCountMoreThanTheStage(nedStage)
	return count >= nedCount
}

func godTortoise_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 2 {
		return false
	}
	principalSuit, ok := sys.principalSuit[custom_id.FourSymbolsTortoise]
	if !ok {
		return false
	}
	nedCount := uint32(args[0])
	nedStage := uint32(args[1])
	count := principalSuit.GetCountMoreThanTheStage(nedStage)
	return count >= nedCount
}

func cimeliaFourthly_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiCimelia).(*CimeliaSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 1 {
		return false
	}
	nedCount := uint32(args[0])
	val, ok := sys.GetCimeliaData(CimeliaFourthly)
	if !ok {
		return false
	}
	return val.Value >= nedCount
}

func cimeliaThird_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiCimelia).(*CimeliaSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 1 {
		return false
	}
	nedCount := uint32(args[0])
	val, ok := sys.GetCimeliaData(CimeliaThird)
	if !ok {
		return false
	}
	return val.Value >= nedCount
}

func cimeliaSecond_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiCimelia).(*CimeliaSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 1 {
		return false
	}
	nedCount := uint32(args[0])
	val, ok := sys.GetCimeliaData(CimeliaSecond)
	if !ok {
		return false
	}
	return val.Value >= nedCount
}

func cimeliaFirst_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiCimelia).(*CimeliaSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 1 {
		return false
	}
	nedCount := uint32(args[0])
	val, ok := sys.GetCimeliaData(CimeliaFirst)
	if !ok {
		return false
	}
	return val.Value >= nedCount
}

func cimeliaFirst_NewJingJie(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 1 {
		return false
	}
	nedCount := uint32(args[0])
	val := sys.GetData()
	if !ok {
		return false
	}
	return val.Level >= nedCount
}

func newFaBao_AllLv(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiNewFabao).(*FaBaoSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) == 0 {
		return false
	}

	state := sys.state()
	var allLv uint32
	for _, bao := range state.FaBaoMap {
		allLv += bao.Lv
	}
	return allLv >= uint32(args[0])
}

func dragonEqu_ReachStandarMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiDragon).(*DragonSys)
	if !ok || !sys.IsOpen() {
		return false
	}
	if len(args) == 0 {
		return false
	}
	data := sys.GetDragonEqData()
	if data == nil || len(data.Equips) == 0 {
		return false
	}
	minStage := math.MaxUint32
	for _, itemId := range data.Equips {
		conf := jsondata.GetItemConfig(itemId)
		if conf == nil {
			continue
		}
		if int(conf.Stage) < minStage {
			minStage = int(conf.Stage)
		}
	}
	if minStage == math.MaxUint32 {
		return false
	}
	return minStage >= int(args[0])
}

func destinedFaBao_AllLv(player iface.IPlayer, args []int64) bool {
	if len(args) == 0 {
		return false
	}

	obj := player.GetSysObj(sysdef.SiDestinedFaBao)
	if obj == nil || !obj.IsOpen() {
		return false
	}

	sys, ok := obj.(*DestinedFaBaoSys)
	if !ok {
		return false
	}

	data := sys.getData()
	return data.Lv >= uint32(args[0])
}

func destinedFaBao_SuckBlood(player iface.IPlayer, args []int64) bool {
	if len(args) == 0 {
		return false
	}
	valueAlias := player.GetFightAttr(attrdef.DestinedFaBaoSuckBlood)
	valueAlias = valueAlias * (10000 + player.GetFightAttr(attrdef.DestinedFaBaoSuckBloodRate)) / 10000
	return valueAlias >= args[0]
}

func destinedFaBao_DrawTimes(player iface.IPlayer, args []int64) bool {
	if len(args) < 2 {
		return false
	}

	needTimes, yyId := args[0], args[1]

	yyData := player.GetBinaryData().YyData

	if yyData.EvolutionFaBaoDraw == nil {
		return false
	}
	data := yyData.EvolutionFaBaoDraw[uint32(yyId)]

	if data.LotteryData == nil {
		return false
	}

	return data != nil && int64(data.LotteryData.Times) >= needTimes
}

func dragong_DrawTimes(player iface.IPlayer, args []int64) bool {
	if len(args) < 2 {
		return false
	}

	needTimes, yyId := args[0], args[1]

	yyData := player.GetBinaryData().YyData

	if yyData.EvolutionDragonDraw == nil {
		return false
	}
	data := yyData.EvolutionDragonDraw[uint32(yyId)]

	if data.LotteryData == nil {
		return false
	}

	return data != nil && int64(data.LotteryData.Times) >= needTimes
}

func soulHaloSuitQuality_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 1 {
		return false
	}

	quality := uint32(args[0])

	var maxQuality uint32
	if s, ok := player.GetSysObj(sysdef.SiSoulHalo).(*SoulHaloSys); ok && s.IsOpen() {
		maxQuality = s.getSuitMaxQuality()
	}

	return maxQuality >= quality
}

func sponsorGift_ReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	s, ok := player.GetSysObj(sysdef.SiSponsorGift).(*SponsorGift)
	if !ok || !s.IsOpen() {
		return false
	}
	for _, cfgId := range args {
		if !s.IsBuyGift(uint32(cfgId)) {
			return false
		}
	}
	return true
}

func lingBaoRiderReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiRider).(*RiderSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 2 {
		return false
	}
	nedCount := uint32(args[0])
	nedStage := uint32(args[1])
	count := sys.deputySuit.GetCountMoreThanTheStage(nedStage)
	return count >= nedCount
}

func lingBaoWingReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFairyWing).(*FairyWingSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 2 {
		return false
	}
	nedCount := uint32(args[0])
	nedStage := uint32(args[1])
	count := sys.deputySuit.GetCountMoreThanTheStage(nedStage)
	return count >= nedCount
}

func lingBaoDragonReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 2 {
		return false
	}
	principalSuit, ok := sys.deputySuit[custom_id.FourSymbolsDragon]
	if !ok {
		return false
	}
	nedCount := uint32(args[0])
	nedStage := uint32(args[1])
	count := principalSuit.GetCountMoreThanTheStage(nedStage)
	return count >= nedCount
}

func lingBaoTigerReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 2 {
		return false
	}
	principalSuit, ok := sys.deputySuit[custom_id.FourSymbolsTiger]
	if !ok {
		return false
	}
	nedCount := uint32(args[0])
	nedStage := uint32(args[1])
	count := principalSuit.GetCountMoreThanTheStage(nedStage)
	return count >= nedCount
}

func lingBaoRosefinchReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 2 {
		return false
	}
	principalSuit, ok := sys.deputySuit[custom_id.FourSymbolsRosefinch]
	if !ok {
		return false
	}
	nedCount := uint32(args[0])
	nedStage := uint32(args[1])
	count := principalSuit.GetCountMoreThanTheStage(nedStage)
	return count >= nedCount
}

func lingBaoTortoiseReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if len(args) < 2 {
		return false
	}
	principalSuit, ok := sys.deputySuit[custom_id.FourSymbolsTortoise]
	if !ok {
		return false
	}
	nedCount := uint32(args[0])
	nedStage := uint32(args[1])
	count := principalSuit.GetCountMoreThanTheStage(nedStage)
	return count >= nedCount
}

func greatGiftReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 1 {
		return false
	}
	return utils.SliceContainsUint32(player.GetBinaryData().GreatGiftBuyStatus, uint32(args[0]))
}

func battleSoulLvReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 1 {
		return false
	}

	sys, ok := player.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok || !sys.IsOpen() {
		return false
	}
	lv := uint32(args[0])

	var myLv uint32
	if st := sys.getBattleSoulData().ExpLv; nil != st {
		myLv = st.GetLv()
	}

	return myLv >= lv
}

func battleSoulStageReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 2 {
		return false
	}

	sys, ok := player.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok || !sys.IsOpen() {
		return false
	}
	id, stage := uint32(args[0]), uint32(args[1])
	battleSoul, ok := sys.getBattleSoulById(id)
	if !ok {
		return false
	}

	var myStage uint32
	if st := battleSoul.GetExpStage(); nil != st {
		myStage = st.GetLv()
	}

	return myStage >= stage
}

func dragonEquStageReachStandardMissionChecker(player iface.IPlayer, args []int64) bool {
	sys, ok := player.GetSysObj(sysdef.SiDragon).(*DragonSys)
	if !ok || !sys.IsOpen() {
		return false
	}
	if len(args) < 2 {
		return false
	}
	eqData := sys.GetDragonEqData()
	if eqData == nil {
		return false
	}
	nedCount := uint32(args[0])
	nedStage := uint32(args[1])

	var count uint32
	for _, itemId := range eqData.Equips {
		itemConf := jsondata.GetItemConfig(itemId)
		if itemConf == nil {
			continue
		}
		if itemConf.Stage >= nedStage {
			count++
		}
	}
	return count >= nedCount
}

func battleShieldLevelReachStandardChecker(player iface.IPlayer, args []int64) bool {
	s, ok := player.GetSysObj(sysdef.SiBattleShield).(*BattleShieldSys)
	if !ok || !s.IsOpen() {
		return false
	}
	if len(args) < 1 {
		return false
	}
	data := s.getData()
	nedLv := uint32(args[0])
	return data.ExpLv.Lv >= nedLv
}

func battleShieldStageLvReachStandardChecker(player iface.IPlayer, args []int64) bool {
	s, ok := player.GetSysObj(sysdef.SiBattleShield).(*BattleShieldSys)
	if !ok || !s.IsOpen() {
		return false
	}
	if len(args) < 1 {
		return false
	}
	data := s.getData()
	nedLv := uint32(args[0])
	return data.StageLv.Lv >= nedLv
}

func battleShieldTransformStarReachStandardChecker(player iface.IPlayer, args []int64) bool {
	s, ok := player.GetSysObj(sysdef.SiBattleShieldTransform).(*BattleShieldTransformSys)
	if !ok || !s.IsOpen() {
		return false
	}
	if len(args) < 2 {
		return false
	}
	data := s.getData()
	id := args[0]
	dressData := data.Fashions[uint32(id)]
	if dressData == nil {
		return false
	}
	nedLv := args[1]
	return dressData.Star >= uint32(nedLv)
}
func battleShieldTransformStageReachStandardChecker(player iface.IPlayer, args []int64) bool {
	s, ok := player.GetSysObj(sysdef.SiBattleShieldTransform).(*BattleShieldTransformSys)
	if !ok || !s.IsOpen() {
		return false
	}
	if len(args) < 2 {
		return false
	}
	data := s.getData()
	id := args[0]
	stage := data.FashionStageMap[uint32(id)]
	nedLv := args[1]
	return stage >= uint32(nedLv)
}

func fairySwordStageReachStandardChecker(player iface.IPlayer, args []int64) bool {
	s, ok := player.GetSysObj(sysdef.SiFairySword).(*FairySwordSys)
	if !ok || !s.IsOpen() {
		return false
	}
	if len(args) < 2 {
		return false
	}

	pos := uint32(args[0])
	stage := uint32(args[1])

	posData := s.getPosData(pos)
	if posData.Equip == nil {
		return false
	}

	equip := posData.Equip
	itemConf := jsondata.GetItemConfig(equip.ItemId)
	if itemConf == nil {
		return false
	}
	return itemConf.Stage >= stage
}

func openServerDayReachStandardChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 1 {
		return false
	}
	day := uint32(args[0])
	return gshare.GetOpenServerDay() >= day
}

func sectHallLvReachStandardChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 1 {
		return false
	}

	s, ok := player.GetSysObj(sysdef.SiSectHall).(*SectHall)
	if !ok || !s.IsOpen() {
		return false
	}

	lv := uint32(args[0])
	sectHallLv := s.getGlobalData().Level
	return sectHallLv >= lv
}

func sectCultivateReachStandardChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 2 {
		return false
	}

	s, ok := player.GetSysObj(sysdef.SiSectCultivate).(*SectCultivateSys)
	if !ok || !s.IsOpen() {
		return false
	}

	id := uint32(args[0])
	lv := uint32(args[1])

	return s.GetLevel(id) >= lv
}

func soulHaloPowerReachStandardChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 1 {
		return false
	}

	s, ok := player.GetSysObj(sysdef.SiSoulHalo).(*SoulHaloSys)
	if !ok || !s.IsOpen() {
		return false
	}

	power := args[0]

	return power <= s.getSysPower()
}

func flyingSwordDomainTransformsStar(player iface.IPlayer, args []int64) bool {
	if len(args) < 2 {
		return false
	}

	s, ok := player.GetSysObj(sysdef.SiFlyingSwordDomain).(*FlyingSwordDomainSys)
	if !ok || !s.IsOpen() {
		return false
	}

	transformId := uint32(args[0])
	transformStar := uint32(args[1])

	data := s.getData()
	star, ok := data.Transform[transformId]
	if !ok {
		return false
	}
	return star >= transformStar
}

func soulHaloBreakLvReachStandardChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 2 {
		return false
	}

	s, ok := player.GetSysObj(sysdef.SiSoulHalo).(*SoulHaloSys)
	if !ok || !s.IsOpen() {
		return false
	}

	slot := uint32(args[0])
	breakLv := uint32(args[1])

	equip, err := s.getSoulHaloBySlot(slot)
	if nil != err {
		return false
	}

	return breakLv <= s.getSoulHaloBreakLv(equip)
}

func fashionSetActiveReachStandardChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 2 {
		return false
	}

	sys, ok := player.GetSysObj(sysdef.SiFashionSet).(*FashionSetSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	setId := uint32(args[0])
	count := uint32(args[1])

	conf := jsondata.GetFashionSetConf()
	if _, exist := conf[setId]; !exist {
		return false
	}

	setData := sys.getSetData(setId)
	activeNum := uint32(len(setData.AppearIds))

	return activeNum >= count
}

func pyySummerSurfHitItemReachStandardChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 2 {
		return false
	}

	yyId := uint32(args[0])
	count := uint32(args[1])

	obj := player.GetPYYObj(yyId)
	if nil == obj || !obj.IsOpen() {
		return false
	}

	s, ok := obj.(iface.IPYYSummerSurfDraw)
	if !ok {
		return false
	}

	return s.GetHitItemNums() >= count
}

func faShenStarReachStandardChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 2 {
		return false
	}
	fsId := uint32(args[0])
	star := uint32(args[1])
	sys, ok := player.GetSysObj(sysdef.SiFaShen).(*FaShenSys)
	if !ok || !sys.IsOpen() {
		return false
	}
	data := sys.getData()
	faShen, ok := data.FsMap[fsId]
	if !ok {
		return false
	}
	if faShen.Star < star {
		return false
	}
	return true
}

func yyStoreScoreRankReachStandardChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 2 {
		return false
	}
	yyId := uint32(args[0])
	score := args[1]
	obj := yymgr.GetYYByActId(yyId)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	s, ok := obj.(iface.IYYReachStandardScore)
	if !ok {
		return false
	}

	return s.GetReachStandardScore(player.GetId()) >= score
}

func reachStandardRolePowerChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 1 {
		return false
	}
	power := args[0]
	if len(args) == 1 {
		return player.GetExtraAttr(attrdef.FightValue) >= power
	}
	var totalPower int64
	attrSys := player.GetAttrSys()
	for idx, arg := range args {
		if idx == 0 {
			continue
		}
		totalPower += attrSys.GetSysPower(uint32(arg))
	}
	return totalPower >= power
}

func reachStandardRoleDailyUpPowerChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 1 {
		return false
	}
	power := args[0]
	attrSys := player.GetAttrSys()
	dailyInitPowerInfo := attrSys.GetDailyInitPowerInfo()
	if len(args) == 1 {
		var dailyTotalPower int64
		for _, power := range dailyInitPowerInfo.PowerMap {
			dailyTotalPower += power
		}
		totalPower := player.GetExtraAttr(attrdef.FightValue)
		var diffPower = totalPower - dailyTotalPower
		return diffPower >= power
	}

	var dailyTotalPower int64
	var totalPower int64
	for idx, arg := range args {
		if idx == 0 {
			continue
		}
		totalPower += attrSys.GetSysPower(uint32(arg))
		dailyTotalPower += dailyInitPowerInfo.PowerMap[uint32(arg)]
	}
	var diffPower = totalPower - dailyTotalPower
	return diffPower >= power
}

func reachStandardDailyChargeDiamondChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 1 {
		return false
	}
	diamond := args[0]
	chargeInfo := player.GetBinaryData().GetChargeInfo()
	if chargeInfo == nil {
		return false
	}
	return chargeInfo.DailyChargeDiamond >= uint32(diamond)
}

func reachStandardSponsorPrivilegeChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 1 {
		return false
	}
	privilegeId := args[0]
	obj := player.GetSysObj(sysdef.SiSponsorPrivilege)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	s, ok := obj.(*SponsorPrivilege)
	if !ok {
		return false
	}
	data := s.getData()
	if data.SponsorId < uint32(privilegeId) {
		return false
	}
	return true
}

func reachStandardSoulHaloQiChecker(player iface.IPlayer, args []int64) bool {
	if len(args) < 3 {
		return false
	}
	hunQiId := args[0]
	lv := args[1]
	stage := args[2]
	obj := player.GetSysObj(sysdef.SiSoulHalo)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	s, ok := obj.(*SoulHaloSys)
	if !ok {
		return false
	}
	data := s.GetSoulHaloQiData(uint32(hunQiId))
	if data == nil {
		return false
	}
	if lv != 0 && (data.ExpLv == nil || data.ExpLv.Lv < uint32(lv)) {
		return false
	}
	if stage != 0 && (data.Stage < uint32(stage)) {
		return false
	}
	return true
}

func init() {
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_Level, level_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_StrongTotalLevel, strongLv_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_GemTotalLevel, gemTotalLevel_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_RiderLevel, rider_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_FairyWingLevel, fairyWing_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_FabaoLevel, fabao_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_WeaponLevel, weapon_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_DragonLevel, dragonLevel_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_TigerLevel, tigerLevel_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_Rosefinch, rosefinch_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_Tortoise, tortoise_ReachStandardMissionChecker)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_GodRider, godRider_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_GodWing, godWing_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_GodDragon, godDragon_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_GodTiger, godTiger_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_GodRosefinch, godRosefinch_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_GodTortoise, godTortoise_ReachStandardMissionChecker)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_CimeliaFirst, cimeliaFirst_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_CimeliaSecond, cimeliaSecond_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_CimeliaThird, cimeliaThird_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_CimeliaFourthly, cimeliaFourthly_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_NewJingJie, cimeliaFirst_NewJingJie)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_NewFaoBaoAllLv, newFaBao_AllLv)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_DragonEquLv, dragonEqu_ReachStandarMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_DestinedFaoBaoAllLv, destinedFaBao_AllLv)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_DestinedFaoBaoSuckBlood, destinedFaBao_SuckBlood)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_FaoBaoDrawTimes, destinedFaBao_DrawTimes)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_DraGondrawTimes, dragong_DrawTimes)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_SoulHaloSuitQuality, soulHaloSuitQuality_ReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_SponsorGift, sponsorGift_ReachStandardMissionChecker)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_LingBaoRider, lingBaoRiderReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_LingBaoWing, lingBaoWingReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_LingBaoDragon, lingBaoDragonReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_LingBaoTiger, lingBaoTigerReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_LingBaoRosefinch, lingBaoRosefinchReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_LingBaoTortoise, lingBaoTortoiseReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_DragonEquStage, dragonEquStageReachStandardMissionChecker)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_GreatGift, greatGiftReachStandardMissionChecker)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_BattleSoulLv, battleSoulLvReachStandardMissionChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_BattleSoulStage, battleSoulStageReachStandardMissionChecker)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_BattleShieldLevel, battleShieldLevelReachStandardChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_BattleShieldStageLv, battleShieldStageLvReachStandardChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_BattleShieldTransformStar, battleShieldTransformStarReachStandardChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_BattleShieldTransformStage, battleShieldTransformStageReachStandardChecker)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_FairySwordStage, fairySwordStageReachStandardChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_OpenServerDay, openServerDayReachStandardChecker)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_SectHallLv, sectHallLvReachStandardChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_SectCultivate, sectCultivateReachStandardChecker)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_SoulHaloPower, soulHaloPowerReachStandardChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_FlyingSwordDomainTransformsStar, flyingSwordDomainTransformsStar)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_SoulHaloBreakLv, soulHaloBreakLvReachStandardChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_FashionSetActive, fashionSetActiveReachStandardChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_PYYSummerSurfHitItem, pyySummerSurfHitItemReachStandardChecker)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_FaShenStar, faShenStarReachStandardChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_RolePower, reachStandardRolePowerChecker)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_RoleDailyUpPower, reachStandardRoleDailyUpPowerChecker)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_YYStoreScoreRank, yyStoreScoreRankReachStandardChecker)

	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_DailyChargeDiamond, reachStandardDailyChargeDiamondChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_SponsorPrivilege, reachStandardSponsorPrivilegeChecker)
	RegisterReachStandardMissionChecker(reachconddef.ReachStandard_SoulHaloQi, reachStandardSoulHaloQiChecker)
}
