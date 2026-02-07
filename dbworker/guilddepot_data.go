/**
 * @Author: LvYuMeng
 * @Date: 2023/11/1
 * @Desc:
**/

package dbworker

import (
	"github.com/995933447/elemutil"
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/db"
	"jjyz/base/pb3"
)

type Guilddepot struct {
	Id        uint64
	GuildId   uint64
	SysId     uint8
	SrvId     uint32
	ConfId    uint32
	Count     int64
	Bind      uint8
	Time      uint32
	Pos       uint32
	RandAttr  []byte
	Union1    uint32
	RandAttr2 []byte
	Union2    uint32
	Ext       []byte
}

func (data *Guilddepot) formatItemPool(st *pb3.ItemSt) {
	data.Id = st.GetHandle()
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

func saveNewGuildItem(guildId uint64, items []*pb3.ItemSt) {
	var offset int
	mapHdlToOrigItem := map[uint64]*Guilddepot{}
	for {
		var batch []*Guilddepot
		err := db.OrmEngine.
			Where("guild_id = ?", guildId).
			Limit(1000, offset).
			Find(&batch)
		if err != nil {
			logger.LogError(err.Error())
			return
		}

		offset += len(batch)

		for _, item := range batch {
			mapHdlToOrigItem[item.Id] = item
		}

		if len(batch) < 1000 {
			break
		}
	}

	srvId := GetSrvIdFromGuildId(guildId)

	mapHdlToItem := map[uint64]*Guilddepot{}
	for _, st := range items {
		data := &Guilddepot{}
		data.SrvId = srvId
		data.SysId = sysGuildDepot
		data.GuildId = guildId
		data.formatItemPool(st)
		mapHdlToItem[data.Id] = data
	}

	var (
		upList, addList []*Guilddepot
		delHdlList      []uint64
	)
	for hdl, oriItem := range mapHdlToOrigItem {
		item, ok := mapHdlToItem[hdl]
		if !ok {
			delHdlList = append(delHdlList, oriItem.Id)
			continue
		}
		upList = append(upList, item)
	}

	for hdl, item := range mapHdlToItem {
		if _, ok := mapHdlToOrigItem[hdl]; ok {
			continue
		}
		addList = append(addList, item)
	}

	if len(addList) > 0 {
		if _, err := db.OrmEngine.Insert(&addList); err != nil {
			logger.LogError("save item error %v", err)
		}
	}

	if len(delHdlList) > 0 {
		if _, err := db.OrmEngine.In("id", delHdlList).Delete(&Guilddepot{}); err != nil {
			logger.LogError("save item error %v", err)
		}
	}

	if len(upList) > 0 {
		for _, item := range upList {
			_, err := db.OrmEngine.
				Table("guilddepot").
				Where("id = ?", item.Id).
				Update(elemutil.Struct2MapWithUnderLineKey(item))
			if err != nil {
				logger.LogError(err.Error())
			}
		}
	}
}

func formatGuildItemSt(item *Guilddepot) *pb3.ItemSt {
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
		Handle:  item.Id,
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

func loadNewGuildItems(guildId uint64) ([]*pb3.ItemSt, error) {
	var items []*Guilddepot
	if err := db.OrmEngine.SQL("call loadGuildItemPool(?)", guildId).Find(&items); nil != err {
		logger.LogError("%s", err)
		return nil, err
	}

	ret := make([]*pb3.ItemSt, 0, len(items))
	for _, item := range items {
		itemSt := formatGuildItemSt(item)
		if nil != itemSt {
			ret = append(ret, itemSt)
		}
	}

	return ret, nil
}
