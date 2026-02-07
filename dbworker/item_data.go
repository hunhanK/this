/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/30 15:52
 */

package dbworker

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"github.com/995933447/elemutil"
	"jjyz/base"
	"jjyz/base/db"
	"jjyz/base/pb3"
	"time"

	"github.com/gzjjyz/logger"
)

const (
	sysBag                  = 1  // 通用背包
	sysEquip                = 2  // 穿戴装备
	sysDepot                = 3  // 玩家仓库
	sysGuildDepot           = 4  // 帮会仓库
	sysImmortalSoul         = 5  // 战魂装备
	sysEdict                = 6  // 敕令装备
	sysFairyBag             = 7  // 仙灵背包
	sysMarryEquip           = 8  // 结婚装备
	sysGodBeastBag          = 9  // 神兽装备背包
	sysFairyEquip           = 10 // 仙灵装备背包
	sysBattleSoulGodEquip   = 11 // 武魂神饰背包
	sysMementoBag           = 12 // 古宝背包
	sysFairySword           = 13 // 仙剑背包
	sysFairySpirit          = 14 // 仙灵灵装背包
	sysHolyEquip            = 15 // 圣装背包
	sysFlyingSwordEquip     = 16 // 飞剑玉符背包
	sysSourceSoul           = 17 // 源魂背包
	sysBlood                = 18 // 血脉
	sysBloodEqu             = 19 // 魂装(血脉装备)
	sysSmith                = 20 // 铁匠背包
	sysFeatherEqu           = 21 // 羽装
	sysDomainSoul           = 22 // 领域域灵
	sysDomainEye            = 23 // 领域域眼
	sysWarPaintFairyWing    = 24 // 战纹-仙翼
	sysWarPaintGodWeapon    = 25 // 战纹-神兵
	sysWarPaintBattleShield = 26 // 战纹-战盾
	sysPrivilegeDepot       = 27 // 特权仓库
	sysSoulHaloSkeleton     = 28 //魂环器骸
)

type itempool struct {
	Id        uint64
	ActorId   uint64
	SysId     uint8
	ItemGuid  uint64
	ConfId    uint32
	Count     int64
	Bind      uint8
	Time      uint32
	Pos       uint32
	RandAttr  []byte
	Union1    uint32
	DropInfo  []byte
	RandAttr2 []byte
	Union2    uint32
	Ext       []byte
	Md5       string
}

// HashMD5 生成 MD5 哈希（返回字符串）
func (data *itempool) HashMD5() string {
	h := md5.New()
	buf := make([]byte, 8)

	binary.LittleEndian.PutUint64(buf, data.ActorId)
	h.Write(buf)
	h.Write([]byte{data.SysId})
	binary.LittleEndian.PutUint64(buf, data.ItemGuid)
	h.Write(buf)
	binary.LittleEndian.PutUint32(buf[:4], data.ConfId)
	h.Write(buf[:4])
	binary.LittleEndian.PutUint64(buf, uint64(data.Count))
	h.Write(buf)
	h.Write([]byte{data.Bind})
	binary.LittleEndian.PutUint32(buf[:4], data.Time)
	h.Write(buf[:4])
	binary.LittleEndian.PutUint32(buf[:4], data.Pos)
	h.Write(buf[:4])

	h.Write(data.RandAttr)
	binary.LittleEndian.PutUint32(buf[:4], data.Union1)
	h.Write(buf[:4])
	h.Write(data.DropInfo)
	h.Write(data.RandAttr2)
	binary.LittleEndian.PutUint32(buf[:4], data.Union2)
	h.Write(buf[:4])
	h.Write(data.Ext)

	return hex.EncodeToString(h.Sum(nil))
}

func transfer(actorId uint64, sysId uint8, dest map[uint64]*itempool, items []*pb3.ItemSt) {
	for _, st := range items {
		data := &itempool{}
		data.ActorId = actorId
		data.SysId = sysId
		data.formatItemPool(st)
		data.Md5 = data.HashMD5()
		dest[data.ItemGuid] = data
	}
}

func saveItemPool(actorId uint64, pool *pb3.ItemPool) error {
	var offset int
	curItemMap := map[uint64]*itempool{}
	for {
		var batch []*itempool
		err := db.OrmEngine.Where("actor_id = ?", actorId).Limit(1000, offset).Find(&batch)
		if err != nil {
			logger.LogError(err.Error())
			return err
		}

		offset += len(batch)

		for _, item := range batch {
			curItemMap[item.ItemGuid] = item
		}

		if len(batch) < 1000 {
			break
		}
	}

	mapHdlToItem := map[uint64]*itempool{}

	transfer(actorId, sysBag, mapHdlToItem, pool.Bags)
	transfer(actorId, sysEquip, mapHdlToItem, pool.Equips)
	transfer(actorId, sysDepot, mapHdlToItem, pool.DepotItems)
	transfer(actorId, sysImmortalSoul, mapHdlToItem, pool.ImmortalSouls)
	transfer(actorId, sysEdict, mapHdlToItem, pool.Edicts)
	transfer(actorId, sysFairyBag, mapHdlToItem, pool.FairyBag)
	transfer(actorId, sysMarryEquip, mapHdlToItem, pool.MarryEquips)
	transfer(actorId, sysGodBeastBag, mapHdlToItem, pool.GodBeastBag)
	transfer(actorId, sysFairyEquip, mapHdlToItem, pool.FairyEquips)
	transfer(actorId, sysBattleSoulGodEquip, mapHdlToItem, pool.BattleSoulGodEquips)
	transfer(actorId, sysMementoBag, mapHdlToItem, pool.MementoBag)
	transfer(actorId, sysFairySword, mapHdlToItem, pool.FairySwords)
	transfer(actorId, sysFairySpirit, mapHdlToItem, pool.FairySpirits)
	transfer(actorId, sysHolyEquip, mapHdlToItem, pool.HolyEquips)
	transfer(actorId, sysFlyingSwordEquip, mapHdlToItem, pool.FlyingSwordEquips)
	transfer(actorId, sysSourceSoul, mapHdlToItem, pool.SourceSouls)
	transfer(actorId, sysBlood, mapHdlToItem, pool.BloodBag)
	transfer(actorId, sysBloodEqu, mapHdlToItem, pool.BloodEquBag)
	transfer(actorId, sysSmith, mapHdlToItem, pool.SmithBag)
	transfer(actorId, sysFeatherEqu, mapHdlToItem, pool.FeatherEquBag)
	transfer(actorId, sysDomainSoul, mapHdlToItem, pool.DomainSoulBag)
	transfer(actorId, sysDomainEye, mapHdlToItem, pool.DomainEyeBag)
	transfer(actorId, sysWarPaintFairyWing, mapHdlToItem, pool.WarPaintFairyWingBag)
	transfer(actorId, sysWarPaintGodWeapon, mapHdlToItem, pool.WarPaintGodWeaponBag)
	transfer(actorId, sysWarPaintBattleShield, mapHdlToItem, pool.WarPaintBattleShieldBag)
	transfer(actorId, sysPrivilegeDepot, mapHdlToItem, pool.PrivilegeDepot)
	transfer(actorId, sysSoulHaloSkeleton, mapHdlToItem, pool.SoulHaloSkeletonBag)

	var (
		upList, addList     []*itempool
		delIdxList          []uint64
		updateHdl2OriginMd5 = make(map[uint64]string)
	)
	for hdl, oriItem := range curItemMap {
		item, ok := mapHdlToItem[hdl]
		if !ok {
			delIdxList = append(delIdxList, oriItem.Id)
			continue
		}
		item.Id = oriItem.Id
		if item.Md5 == oriItem.Md5 {
			continue
		}
		upList = append(upList, item)
		updateHdl2OriginMd5[hdl] = oriItem.Md5
	}

	for hdl, item := range mapHdlToItem {
		if _, ok := curItemMap[hdl]; ok {
			continue
		}
		addList = append(addList, item)
	}

	if len(addList) > 0 {
		if _, err := db.OrmEngine.Insert(&addList); err != nil {
			logger.LogError("add actor(id:%d) item error %v", actorId, err)
		}
	}

	if len(delIdxList) > 0 {
		if _, err := db.OrmEngine.Where("actor_id = ?", actorId).In("id", delIdxList).Delete(&itempool{}); err != nil {
			logger.LogError("del actor(id:%d) item error %v", actorId, err)
		}
	}

	if len(upList) > 0 {
		for _, item := range upList {
			_, err := db.OrmEngine.
				Table("itempool").
				Where("id = ?", item.Id).
				Where("md5='' or md5=?", updateHdl2OriginMd5[item.ItemGuid]).
				Update(elemutil.Struct2MapWithUnderLineKey(item))
			if err != nil {
				logger.LogError("update actor(id:%d) item error %v", actorId, err)
			}
		}
	}
	return nil
}

func (data *itempool) formatItemPool(st *pb3.ItemSt) {
	data.ItemGuid = st.GetHandle()
	data.ConfId = st.GetItemId()
	data.Count = st.GetCount()
	if st.GetBind() {
		data.Bind = 1
	} else {
		data.Bind = 0
	}

	data.Time = st.GetTimeOut()
	data.Pos = st.GetPos()
	data.Union1 = st.GetUnion1()
	data.Union2 = st.GetUnion2()
	data.RandAttr = base.Pb2Byte(&pb3.PbAttrSt{Attrs: st.Attrs})
	data.RandAttr2 = base.Pb2Byte(&pb3.PbAttrSt{Attrs: st.Attrs2})
	data.Ext = base.Pb2Byte(st.GetExt())
}

func formatItemSt(item *itempool) *pb3.ItemSt {
	pbAttrSt := pb3.PbAttrSt{}
	pb3.Unmarshal(item.RandAttr, &pbAttrSt)

	pbAttrSt2 := pb3.PbAttrSt{}
	pb3.Unmarshal(item.RandAttr2, &pbAttrSt2)

	ext := &pb3.ItemExt{}
	err := pb3.Unmarshal(item.Ext, ext)
	if err != nil {
		logger.LogError("itemExt parse error! err=%v", err)
		return nil
	}

	itemSt := &pb3.ItemSt{
		Handle:  item.ItemGuid,
		ItemId:  item.ConfId,
		Count:   item.Count,
		Bind:    item.Bind == 1,
		TimeOut: item.Time,
		Pos:     item.Pos,
		Union1:  item.Union1,
		Union2:  item.Union2,
		Attrs:   pbAttrSt.Attrs,
		Attrs2:  pbAttrSt2.Attrs,
		Ext:     ext,
	}
	return itemSt
}

func loadAllItem(actorData *pb3.PlayerMainData, actorId uint64) {
	start := time.Now()
	defer func() {
		logger.LogInfo("actor[%d] load all item cost:%v", actorId, time.Since(start))
	}()
	var items []*itempool
	if err := db.OrmEngine.Where("actor_id = ?", actorId).Find(&items); nil != err {
		logger.LogError("%d load all item error %s", actorId, err)
		return
	}

	for _, item := range items {
		pbItem := formatItemSt(item)
		switch item.SysId {
		case sysBag:
			actorData.ItemPool.Bags = append(actorData.ItemPool.Bags, pbItem)
		case sysEquip:
			actorData.ItemPool.Equips = append(actorData.ItemPool.Equips, pbItem)
		case sysDepot:
			actorData.ItemPool.DepotItems = append(actorData.ItemPool.DepotItems, pbItem)
		case sysImmortalSoul:
			actorData.ItemPool.ImmortalSouls = append(actorData.ItemPool.ImmortalSouls, pbItem)
		case sysEdict:
			actorData.ItemPool.Edicts = append(actorData.ItemPool.Edicts, pbItem)
		case sysFairyBag:
			actorData.ItemPool.FairyBag = append(actorData.ItemPool.FairyBag, pbItem)
		case sysMarryEquip:
			actorData.ItemPool.MarryEquips = append(actorData.ItemPool.MarryEquips, pbItem)
		case sysGodBeastBag:
			actorData.ItemPool.GodBeastBag = append(actorData.ItemPool.GodBeastBag, pbItem)
		case sysFairyEquip:
			actorData.ItemPool.FairyEquips = append(actorData.ItemPool.FairyEquips, pbItem)
		case sysBattleSoulGodEquip:
			actorData.ItemPool.BattleSoulGodEquips = append(actorData.ItemPool.BattleSoulGodEquips, pbItem)
		case sysMementoBag:
			actorData.ItemPool.MementoBag = append(actorData.ItemPool.MementoBag, pbItem)
		case sysFairySword:
			actorData.ItemPool.FairySwords = append(actorData.ItemPool.FairySwords, pbItem)
		case sysFairySpirit:
			actorData.ItemPool.FairySpirits = append(actorData.ItemPool.FairySpirits, pbItem)
		case sysHolyEquip:
			actorData.ItemPool.HolyEquips = append(actorData.ItemPool.HolyEquips, pbItem)
		case sysFlyingSwordEquip:
			actorData.ItemPool.FlyingSwordEquips = append(actorData.ItemPool.FlyingSwordEquips, pbItem)
		case sysSourceSoul:
			actorData.ItemPool.SourceSouls = append(actorData.ItemPool.SourceSouls, pbItem)
		case sysBlood:
			actorData.ItemPool.BloodBag = append(actorData.ItemPool.BloodBag, pbItem)
		case sysBloodEqu:
			actorData.ItemPool.BloodEquBag = append(actorData.ItemPool.BloodEquBag, pbItem)
		case sysSmith:
			actorData.ItemPool.SmithBag = append(actorData.ItemPool.SmithBag, pbItem)
		case sysFeatherEqu:
			actorData.ItemPool.FeatherEquBag = append(actorData.ItemPool.FeatherEquBag, pbItem)
		case sysDomainSoul:
			actorData.ItemPool.DomainSoulBag = append(actorData.ItemPool.DomainSoulBag, pbItem)
		case sysDomainEye:
			actorData.ItemPool.DomainEyeBag = append(actorData.ItemPool.DomainEyeBag, pbItem)
		case sysWarPaintFairyWing:
			actorData.ItemPool.WarPaintFairyWingBag = append(actorData.ItemPool.WarPaintFairyWingBag, pbItem)
		case sysWarPaintGodWeapon:
			actorData.ItemPool.WarPaintGodWeaponBag = append(actorData.ItemPool.WarPaintGodWeaponBag, pbItem)
		case sysWarPaintBattleShield:
			actorData.ItemPool.WarPaintBattleShieldBag = append(actorData.ItemPool.WarPaintBattleShieldBag, pbItem)
		case sysPrivilegeDepot:
			actorData.ItemPool.PrivilegeDepot = append(actorData.ItemPool.PrivilegeDepot, pbItem)
		case sysSoulHaloSkeleton:
			actorData.ItemPool.SoulHaloSkeletonBag = append(actorData.ItemPool.SoulHaloSkeletonBag, pbItem)
		}
	}
}
