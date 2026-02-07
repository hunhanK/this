package iface

type IQuestTargetSys interface {
	OnQuestEvent(actor IPlayer, qt uint32, id, count uint32, add bool)
	CalcQuestTargetByRange(actor IPlayer, qtt, tVal, preVal, qtype uint32)
	CalcQuestTargetByRange2(actor IPlayer, qt uint32, args ...interface{})
}

type IQuestGM interface {
	GMReAcceptQuest(questId uint32)
	GMDelQuest(questId uint32)
}
