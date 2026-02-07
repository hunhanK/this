package gshare

type (
	// ActorMailSt 玩家邮件结构
	ActorMailSt struct {
		ActorId  uint64
		ConfId   uint16
		MailId   uint64
		MailType int8
		Status   uint32
		SendTick uint32
		SendName string
		Title    string
		Content  string
		AwardStr string
		UserItem []byte
	}

	// ActorMailStEx ActorMailSt 玩家邮件结构
	ActorMailStEx struct {
		ActorId  uint64
		ConfId   uint16
		MailId   uint64
		Type     int8
		Status   uint32
		SendTick uint32
		SendName string
		Title    string
		Content  string
		AwardStr string
		UserItem []byte
	}
)
