package entity

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/custom_id/sysdef"
	"jjyz/gameserver/logicworker/actorsystem"
)

func (player *Player) GetMoneySys() *actorsystem.MoneySys {
	if sys, ok := player.GetSysObj(sysdef.SiMoney).(*actorsystem.MoneySys); ok {
		return sys
	}
	return nil
}

func (player *Player) GetEquipSys() *actorsystem.EquipSystem {
	if sys, ok := player.GetSysObj(sysdef.SiEquip).(*actorsystem.EquipSystem); ok {
		return sys
	}
	return nil
}

func (player *Player) GetLevelSys() *actorsystem.LevelSys {
	if sys, ok := player.GetSysObj(sysdef.SiLevel).(*actorsystem.LevelSys); ok {
		return sys
	}
	return nil
}

func (player *Player) GetSkillSys() *actorsystem.SkillSys {
	if sys, ok := player.GetSysObj(sysdef.SiSkill).(*actorsystem.SkillSys); ok {
		return sys
	}
	return nil
}

func (player *Player) GetMailSys() *actorsystem.MailSys {
	if sys, ok := player.GetSysObj(sysdef.SiMail).(*actorsystem.MailSys); ok {
		return sys
	}
	return nil
}

func (player *Player) GetTrialActiveSys() *actorsystem.TrialActiveSys {
	if sys, ok := player.GetSysObj(sysdef.SiTrialActive).(*actorsystem.TrialActiveSys); ok {
		return sys
	}
	return nil
}

func (player *Player) IsOpenNotified(sysId uint32) bool {
	binary := player.GetBinaryData()
	if nil == binary.FuncOpenInfo || nil == binary.FuncOpenInfo.FuncOpenStatus {
		return false
	}
	if sysId < 0 {
		return true
	}

	idxInt := sysId / 32
	idxByte := sysId % 32

	flag := binary.FuncOpenInfo.FuncOpenStatus[idxInt]

	return utils.IsSetBit(flag, idxByte)
}
