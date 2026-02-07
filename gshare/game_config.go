package gshare

import (
	"io/ioutil"

	"github.com/gzjjyz/srvlib/utils"
	jsoniter "github.com/json-iterator/go"
)

// MysqlConf mysql数据库配置
type MysqlConf struct {
	Host    string
	Port    int
	User    string
	Pwd     string
	DB      string
	Charset string
}

type gameConfig struct {
	PfId             uint32 // 平台id
	SrvId            uint32 // 服务器id
	LogLevel         int    // 日志等级
	LocalFightSrv    string // 本服战斗服配置
	GateHost         string // 网关地址
	MySql            *MysqlConf
	LogOnConsole     bool   `json:"logOnConsole"`     // 是否在控制台打印日志
	SendErrorMsg     bool   `json:"SendErrorMsg"`     // 是否发送报错信息
	BackStageChatUrl string `json:"backStageChatUrl"` // 后台敏感字url
	BackStageChatKey string `json:"backStageChatKey"` // 后台敏感字api_key
	SkipSdkReport    bool   `json:"skipSdkReport"`    // 跳过sdk上报
	SkipWordMonitor  bool   `json:"skipWordMonitor"`  // 跳过敏感词检测
}

var GameConf *gameConfig //服务器配置

func (config *gameConfig) load(file string) error {
	if 0 == len(file) {
		file = utils.GetCurrentDir() + "gamesrv.json"
	}
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	if err := jsoniter.Unmarshal(data, config); err != nil {
		return err
	}

	return nil
}

// LoadGameConf 加载配置
func LoadGameConf(file string) (*gameConfig, error) {
	GameConf = new(gameConfig)
	if err := GameConf.load(file); nil != err {
		return nil, err
	}
	return GameConf, nil
}
