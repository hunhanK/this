/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/30 15:51
 */

package dbworker

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/db"
	"jjyz/base/pb3"
)

type Skill struct {
	ActorId uint64 //玩家id
	SkillId uint32 //技能id
	Level   uint32 //技能等级
	CdTime  int64  //cd
	Cd      int64
}

// 保存玩家技能数据
func saveActorSkill(actorId uint64, skills map[uint32]*pb3.SkillInfo) {
	//先删除技能数据
	_, err := db.OrmEngine.Exec("call cleanActorSkill(?)", actorId)
	if nil != err {
		logger.LogError("save skill error!!! error:%v", err)
		return
	}

	length := len(skills)
	if length <= 0 {
		return
	}
	list := make([]*Skill, 0, length)
	for _, st := range skills {
		list = append(list, &Skill{
			ActorId: actorId,
			SkillId: st.Id,
			Level:   st.Level,
			Cd:      st.Cd,
		})
	}

	if _, err := db.OrmEngine.Insert(&list); err != nil {
		logger.LogError("insertActorSkill error %v", err)
	}
}

// 加载技能数据
func loadActorSkill(actorId uint64) ([]*Skill, error) {
	list := make([]*Skill, 0)
	err := db.OrmEngine.Table("skill").Where("actor_id = ?", actorId).Find(&list)

	return list, err
}
