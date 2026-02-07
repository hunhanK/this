package main

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/alarm"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	mysql2 "jjyz/base/db/mysql"
	"jjyz/base/pb3"
	"jjyz/base/pubsub"
	"jjyz/gameserver/dbworker"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/fightworker"
	"jjyz/gameserver/gateworker"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/logicworker"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/objversionworker"
	"jjyz/gameserver/redisworker"
	"jjyz/gameserver/sdkworker"
	"log"
	"math/rand"
	"os"
	"time"

	micro "github.com/gzjjyz/simple-micro"
	"github.com/gzjjyz/srvlib/utils/signal"

	"github.com/995933447/std-go/scan"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

var (
	SRC_VERSION    string // src version
	CONFIG_VERSION string // config version
)

func main() {
	utils.SetDev(scan.OptBoolDefault("dev", false))
	start := time.Now()
	rand.Seed(start.UTC().UnixNano())

	var err error
	conf, err := gshare.LoadGameConf("")
	if nil != err {
		log.Fatalf("load config fail, err: %s", err)
		return
	}

	path := utils.GetCurrentDir() + "log"

	logInstance := logger.InitLogger(logger.WithAppName(base.GameServer.String()), logger.WithScreen(gshare.GameConf.LogOnConsole), logger.WithPath(path), logger.WithLevel(conf.LogLevel))
	logger.SetGlobalSkipFilePath()

	defer func() {
		logger.Flush()
		fmt.Println("logger已刷盘")
	}()

	mysql := conf.MySql
	if err = db.InitOrmMysql(mysql.User, mysql.Pwd, mysql.Host, mysql.Port, mysql.DB, mysql.Charset); nil != err {
		logger.LogFatal("连接mysql数据库失败: %s", err)
	}

	logger.LogInfo("========================Version========================")
	logger.LogInfo("SVN VERSION:%s", SRC_VERSION)
	logger.LogInfo("CONFIG VERSION:%s", CONFIG_VERSION)
	logger.LogInfo("========================Version========================")

	err = micro.InitMeta(scan.OptStrDefault("mc", ""))
	if err != nil {
		logger.LogFatal(err.Error())
	}

	if err = logworker.Init(); nil != err {
		logger.LogFatal("log worker init error! %v", err)
	}

	redisWorker, err := redisworker.NewRedisWorker()
	if nil != err {
		logger.LogFatal("create redis worker failed. %v", err)
	}

	dbWorker, err := dbworker.NewDBWorker()
	if nil != err {
		logger.LogFatal("create db worker failed. %v", err)
	}

	if err = LoadData(); nil != err {
		logger.LogFatal("load data failed. %v", err)
	}

	sdkWorker, err := sdkworker.NewSdkWorker()
	if nil != err {
		logger.LogFatal("create sdk worker failed. %v", err)
	}

	err = objversionworker.Init()
	if nil != err {
		logger.LogFatal("create objversion worker failed. %v", err)
	}

	logicWorker, err := logicworker.NewLogicWorker()
	if nil != err {
		logger.LogFatal("create logic worker failed. %v", err)
	}

	if err = redisWorker.GoStart(); nil != err {
		logger.LogFatal("redis worker start failed. %v", err)
	}

	if err = dbWorker.GoStart(); nil != err {
		logger.LogFatal("db worker start failed. %v", err)
	}

	if err = sdkWorker.GoStart(); nil != err {
		logger.LogFatal("sdk worker start failed. %v", err)
	}

	if err = logicWorker.GoStart(); nil != err {
		logger.LogFatal("logic worker start failed. %v", err)
	}

	fightworker.Startup()
	loadCrossInfo()
	loadMediumCross()
	loadLoadGuildRule()
	gateworker.Startup()

	if err := initPubSub(logInstance); err != nil {
		logger.LogFatal("init pubsub error! %v", err)
	}
	defer pubsub.Close()

	alarm.RegSafeWorker(engine.GetAppId(), engine.GetPfId(), engine.GetServerId(), base.GameServer.String())
	engine.WaitLocalFightConn()
	engine.WaitGateConn()
	alarm.ReportWanFengSrvHealth(engine.GetPfId(), engine.GetServerId(), engine.GetAppId(), alarm.WanFengSrvByGame)
	logger.LogInfo("service start done.耗时:%+v 服务器Id:%v", time.Since(start), conf.SrvId)

	s := <-signal.SignalChan()
	logger.LogInfo("receive signal %d", s)

	_ = logicWorker.Close()
	gateworker.OnSrvBeforeStop()
	gateworker.OnSrvStop()
	_ = sdkWorker.Close()
	_ = dbWorker.Close()
	_ = redisWorker.Close()

	objversionworker.Flush()
	logworker.Flush()
	logger.LogWarn("[%d]服务器关闭成功,总运行时长:[%s]!!!", os.Getpid(), time.Since(start))

}

func loadGlobalVar(mainSrvId uint32) error {
	// 加载globalVar
	var datas []mysql2.GlobalVar
	if err := db.OrmEngine.SQL("call loadAllGlobalVar()").Find(&datas); nil != err {
		logger.LogError("load global error! %v", err)
		return err
	}

	for _, data := range datas {
		global := &pb3.GlobalVar{}
		if err := pb3.Unmarshal(pb3.UnCompress(data.BinaryData), global); nil != err {
			logger.LogFatal("load global_%d error! %v", data.ServerId, err)
			continue
		}

		if data.ServerId == mainSrvId {
			if global.MergeData == nil {
				global.MergeData = make(map[uint32]uint32)
				mergeTimes := global.MergeTimes
				mergeTimestamp := global.MergeTimestamp

				for i := uint32(1); i <= mergeTimes; i++ {
					global.MergeData[i] = mergeTimestamp - (mergeTimes-i)*(2*custom_id.Week)
				}
			}

			gshare.SetStaticVar(global)
			break
		}
	}

	return nil
}
