package autodb

import "jjyz/base/db/migrate"

var Procedures = []*migrate.ProcedureSt{
	{
		Name: "initdb",
		SQL: `
			create procedure initdb ()
			begin
				delete from constvariables;
				insert into constvariables(actorid_series_bits, actorid_series_mask, actor_mail_max) values(24, 0xFFFFFF, 200);
			end
		`,
	},
	{
		Name: "checkUserValid",
		SQL: `
		CREATE PROCEDURE checkUserValid(in nAccount varchar(64))
		BEGIN
			SELECT user_id, passwd, unix_timestamp(updatetime) as uptime, pwtime, gmlevel, isinvite FROM account WHERE account_name = nAccount;
			UPDATE account SET updatetime=now() WHERE 'account_name' = nAccount;
		END
	`,
	},
	{
		Name: "cliententergame",
		SQL: `
			create procedure cliententergame(in nactorid bigint, in nuserid integer, in ip bigint)
			begin
				declare isexists bigint default 0;
				declare ncharstate integer default 0;
				declare nstatus integer default 0;

				update actors set status = 1 where user_id=nuserid and (status&2)=2 and recovery_time<>0 and (UNIX_TIMESTAMP()-recovery_time>=259200);
				select actor_id, status into isexists, ncharstate from actors where actor_id=nactorid and user_id=nuserid and (status&2)=2 and ban_time<UNIX_TIMESTAMP() limit 1;

				/*是否有这个用户*/
				if (isexists <> 0) then
					/*先改这个账户id下的其他角色的状态，把第三位变成0*/
					update actors set status = (status & ~(1 << 2)) where user_id = nuserid and (status&2)=2 and actor_id <> nactorid;
					/*再改选中的这个角色*/
					update actors set status = (status | 4), lastloginip=ip where user_id = nuserid and (status&2)=2 and actor_id = nactorid;
					update actors set recovery_time = 0 where user_id = nuserid and (status&2)=2 and actor_id = nactorid and recovery_time<>0;
					select status into nstatus from actors where actor_id=nactorid;
					select 1 as ret, nstatus as status;
				else
					select 0 as ret, 0 as status;
				end if;
			end
		`,
	},
	{
		Name: "loadsrvids",
		SQL: `
			create procedure loadsrvids()
			begin
				declare bits int;

				select actorid_series_bits into bits from constvariables;

				select (actor_id >> bits) & 0xFFFF as srvid from actors group by srvid;
			end
		`,
	},
	{
		Name: "loadmaxactoridseries",
		SQL: `
			create procedure loadmaxactoridseries (in serverid integer)
			begin
				declare bits int;
				declare mask int unsigned;

				select actorid_series_bits into bits from constvariables;
				select actorid_series_mask into mask from constvariables;

				select max(actor_id & mask) as max_series from actors where ((actor_id >> bits) & 0xFFFF) = serverid;
			end
		`,
	},
	{
		Name: "loadmaxmailidseries",
		SQL: `
			create procedure loadmaxmailidseries ()
			begin
				select max(mail_id & 0xffffffff) as max_series from actormail where mail_id >> 32 != 0;
			end
		`,
	},
	{
		Name: "loadmaxguilddepotidseries",
		SQL: `
			create procedure loadmaxguilddepotidseries ()
			begin
				select max(id & 0xffffffff) as max_series from guilddepot where id >> 32 != 0;
			end
		`,
	},
	{
		Name: "clientcreatenewactor",
		SQL: `
			create procedure clientcreatenewactor (in nuserid integer,
				in ip bigint,
				in nactorid bigint,
				in sactorname varchar(32),
				in saccountname varchar(80),
				in nsex integer,
				in njob integer,
				in nserverid integer,
				in nditchid integer,
				in nsubditchid integer)
			begin
				declare nowcount integer default 0;
				declare bits int;

				select actorid_series_bits into bits from constvariables;

				set nowcount = (select count(*) from actors where user_id = nuserid and status<>1 and ((actor_id>>bits)&0xffff)=nserverid);
				if nowcount < 5 then
					insert into actors(user_id,actor_id,actor_name,account_name, sex,job,update_time,create_time,lastloginip,status, ditch_id, sub_ditch_id)
					values(nuserid,nactorid,sactorname,saccountname,nsex,njob,now(),now(), ip, 2, nditchid, nsubditchid);
					insert into actor_binary_data(actor_id) values(nactorid);
				end if;
			end
		`,
	},
	{
		Name: "getactorcount",
		SQL: `
			create procedure getactorcount (in nuserid integer, in nserverid integer)
			begin
				declare bits int;

				select actorid_series_bits into bits from constvariables;
				select count(*) as actor_count from actors where status<>1 and user_id = nuserid and ((actor_id>>bits)&0xffff)=nserverid;
			end
		`,
	},
	{
		Name: "loadactorlistbyuserid",
		SQL: `
			create procedure loadactorlistbyuserid (in nuserid integer, in nserverid integer)
			begin
				declare bits int;

				select actorid_series_bits into bits from constvariables;
				update actors set status = 1 where user_id=nuserid and ((actor_id>>bits)&0xffff)=nserverid and (status&2)=2 and recovery_time<>0 and (UNIX_TIMESTAMP()-recovery_time>=259200);
				select actor_id,actor_name,sex,level,job,status,recovery_time,ban_time,circle,appear_info from actors where user_id=nuserid and ((actor_id>>bits)&0xffff)=nserverid and (status & 2)=2;
			end
		`,
	},
	{
		Name: "deleteplayer",
		SQL: `
			create procedure deleteplayer (in nactorid bigint, in nuserid integer)
			begin
				declare curtime int;
				select UNIX_TIMESTAMP() into curtime;
				update actors set recovery_time = curtime where actor_id=nactorid and user_id=nuserid and (status&2)=2;
			end
		`,
	},
	{
		Name: "recoveryplayer",
		SQL: `
			create procedure recoveryplayer (in nactorid bigint, in nuserid integer)
			begin
				declare cnt int;
				select count(*) into cnt from actors where user_id=nuserid and (status&2)=2;
				if cnt < 3 then
					update actors set recovery_time = 0, status = 2 where actor_id=nactorid and user_id=nuserid;
					select 1 as ret;
				else
					select 0 as ret;
				end if;
			end
		`,
	},
	{
		Name: "loadbaseactor",
		SQL: `
			create procedure loadbaseactor(in nactorid bigint)
			begin
				select
					actors.actor_id as actor_id,
					sex,
					job,
					circle,
					level,
					actor_name,
					create_time,
					actor_binary_data.binary_data as binary_data,
					yuan_bao,
					bind_diamonds,
					diamonds,
					ditch_id,
					sub_ditch_id,
					logined_days,
					day_online_time,
					new_day_reset_time,
					exp,
					appear_info,
					last_logout_time,
					fairy_stone,
					actor_binary_data.binary_pb3_data as binary_pb3_data,
					actor_binary_data.medium_binary_pb3_data as medium_binary_pb3_data
				from actors left outer join actor_binary_data
				on actors.actor_id = actor_binary_data.actor_id
				where actors.actor_id = nactorid and (actors.status & 2) = 2;
			end
		`,
	},
	{
		Name: "updateactordata",
		SQL: `
			create procedure updateactordata(
				in nactorid bigint,
				in njob integer,
				in nsex integer,
				in nactorname char(33),
				in nvip integer,
				in ncircle integer,
				in nlevel integer,
				in nyuanbao integer,
				in nbinddiamonds integer,
				in ndiamonds integer,
				in nlastlogouttime integer,
				in nfightvalue bigint,
				in nlastloginip bigint,
				in nlogineddays integer,
				in ndayonlinetime integer,
				in nnewdayresettime integer,
				in nexp bigint,
				in npb3data mediumblob,
				in nappear_info blob,
				in nfairy_stone integer
			)
			begin
				update actors set
					update_time= now(),
					job = njob,
					sex = nsex,
					actor_name = nactorname,
					vip = nvip,
					circle = ncircle,
					level = nlevel,
					yuan_bao = nyuanbao,
					bind_diamonds = nbinddiamonds,
					diamonds = ndiamonds,
					last_logout_time = nlastlogouttime,
					fight_value = nfightvalue,
					lastloginip = nlastloginip,
					logined_days = nlogineddays,
					day_online_time = ndayonlinetime,
					new_day_reset_time = nnewdayresettime,
					exp = nexp,
					appear_info = nappear_info,
					fairy_stone = nfairy_stone
				where actor_id = nactorid limit 1;

				update actor_binary_data set
					medium_binary_pb3_data = npb3data
				where actor_id = nactorid limit 1;
			end
		`,
	},
	{
		Name: "updateactorservermail",
		SQL: `
			create procedure updateactorservermail(in nactorid bigint, in nid bigint)
			begin
				delete from actorservermail where actor_id=nactorid;
				insert into actorservermail(actor_id, mail_id) values(nactorid, nid);
			end
		`,
	},
	{
		Name: "loadactorservermail",
		SQL: `
			create procedure loadactorservermail(in nactorid bigint)
			begin
				select mail_id from actorservermail where actor_id=nactorid limit 1;
			end
		`,
	},
	{
		Name: "loadmaillist",
		SQL: `
			create procedure loadmaillist (in nactorid bigint,in nmailid bigint)
			begin
				declare cnt int;
				declare max int;
				select actor_mail_max into max from constvariables;
				if nmailid = 0 then
					select count(*) into cnt from actormail where actor_id=nactorid and status=3;
					if cnt > max then
						delete from actormail where actor_id=nactorid and status = 3 and mail_date < date_sub(NOW(), interval '7 0:0:0' day_second);
					end if;

					select mail_id,actor_id,conf_id,type,status,send_tick,send_name,title,content,award_str,user_item
					from actormail where actor_id=nactorid and mail_date > date_sub(NOW(), interval '30 0:0:0' day_second) and status<>3 order by send_tick desc;
				else
					select mail_id,actor_id,conf_id,type,status,send_tick,send_name,title,content,award_str,user_item
					from actormail where actor_id=nactorid and mail_id=nmailid and status<>3;
				end if;
			end
		`,
	},
	{
		Name: "addmail",
		SQL: `
			create procedure addmail (in nmailid bigint, in nactorid bigint, in nconfid integer, in ntype integer,  in nstatus integer,
				in nsendtick integer, in nsendname varchar(32), in ntitle varchar(48), in ncontent varchar(1024), in awardStr varchar(512))
			begin
				declare minmailid bigint default 0;
				declare mailcount integer default 0;
				declare max int;
				declare lastlogindatetime datetime;

				select actor_mail_max into max from constvariables;
				select count(*) into mailcount from actormail where actor_id = nactorid and status<>3;
				if (mailcount >= max) then
					select mail_id into minmailid from actormail where actor_id=nactorid and ((ISNULL(user_item)=1 and award_str = "" and status <> 3) or status=2) order by send_tick asc limit 1;
					if (minmailid = 0) then
						select mail_id into minmailid from actormail where actor_id=nactorid and (ISNULL(user_item)=0 or award_str <> "" and status <> 3) order by send_tick asc limit 1;
					end if;
					update actormail set status = 3 where mail_id = minmailid;
				end if;

				# 七天不登录的需要清理邮件
				select lastlogintime into lastlogindatetime from actors where actor_id = nactorid;
				if lastlogindatetime < date_sub(NOW(), interval '7 0:0:0' day_second) then
					delete from actormail where actor_id=nactorid and status=3 and mail_date < date_sub(NOW(), interval '3 0:0:0' day_second);
				end if;

				insert into actormail(mail_id, actor_id, conf_id, type, status, send_tick, send_name, title, content, award_str, mail_date)

				values(nmailid, nactorid, nconfid, ntype, nstatus, nsendtick, nsendname, ntitle, ncontent, awardStr, NOW());

				select minmailid;
			end
		`,
	},
	{
		Name: "adduseritemmail",
		SQL: `
			create procedure adduseritemmail (in nmailid bigint, in nactorid bigint, in nconfid integer,
				in ntype integer, in nstatus integer, in nsendtick integer,
				in nsendname varchar(32), in ntitle varchar(48), in ncontent varchar(1024),
				in buseritem blob
			)
			begin
				declare minmailid bigint default 0;
				declare mailcount integer default 0;
				declare max int;
				declare lastlogindatetime datetime;

				select actor_mail_max into max from constvariables;
				select count(*) into mailcount from actormail where actor_id = nactorid and status<>3;
				if (mailcount >= max) then
					select mail_id into minmailid from actormail where actor_id = nactorid and ((ISNULL(user_item)=1 and award_str="" and status <> 3) or status=2) order by send_tick asc limit 1;
                    if (minmailid = 0) then
						select mail_id into minmailid from actormail where actor_id=nactorid and (ISNULL(user_item)=0 or award_str <> "" and status <> 3) order by send_tick asc limit 1;
					end if;
					update actormail set status = 3 where mail_id = minmailid;
				end if;

				# 七天不登录的需要清理邮件
				select lastlogintime into lastlogindatetime from actors where actor_id = nactorid;
				if lastlogindatetime < date_sub(NOW(), interval '7 0:0:0' day_second) then
					delete from actormail where actor_id=nactorid and status=3 and mail_date < date_sub(NOW(), interval '3 0:0:0' day_second);
				end if;

				insert into actormail(mail_id, actor_id, conf_id, type, status, send_tick, send_name, title, content, user_item, mail_date)
				values(nmailid, nactorid, nconfid, ntype, nstatus, nsendtick, nsendname, ntitle, ncontent, buseritem, NOW());

				select minmailid;
			end
		`,
	},
	{
		Name: "updatemailstatus",
		SQL: `
			create procedure updatemailstatus (IN nactorid bigint, IN nmsgid bigint, IN mailstatus integer)
			begin
				update actormail set status=mailstatus where actor_id=nactorid and mail_id=nmsgid;
			end
		`,
	},
	{
		Name: "deletemail",
		SQL: `
			create procedure deletemail (IN nactorid bigint, IN nmsgid bigint)
			begin
				update actormail set status=3 where mail_id=nmsgid and actor_id=nactorid;
			end
		`,
	},
	{
		Name: "delgmcmd",
		SQL: `
			create procedure delgmcmd (in nid integer)
			begin
				update gmcmd set deltime=UNIX_TIMESTAMP() + 300 where id=nid;
			end
		`,
	},
	{
		Name: "delGuildItemPool",
		SQL: `
			CREATE PROCEDURE delGuildItemPool (IN nid BIGINT)
			BEGIN
				DELETE FROM itempool WHERE guild_id = nid;
			END
		`,
	},
	{
		Name: "loadGuildItemPool",
		SQL: `
			CREATE PROCEDURE loadGuildItemPool (IN guildId BIGINT)
			BEGIN
				SELECT * FROM guilddepot WHERE guild_id = guildId;
			END
		`,
	},
	{
		Name: "cleanActorSkill",
		SQL: `
			CREATE PROCEDURE cleanActorSkill (in actorId bigint)
			BEGIN
				DELETE FROM skill WHERE actor_id = actorId;
			END
		`,
	},
	{
		Name: "loadDeleteActorIds",
		SQL: `
			CREATE PROCEDURE loadDeleteActorIds()
			BEGIN
				SELECT * FROM tmp_table;
			END
		`,
	},
	{
		Name: "loadGuildSeries",
		SQL: `
			CREATE PROCEDURE loadGuildSeries (IN server_id INTEGER)
			BEGIN
				declare series bigint;
				select max(guild_id) into series from guildlist where srv_id = server_id;
				select(series & 0xFFFFF) as series;
			END
		`,
	},
	{
		Name: "loadGuildData",
		SQL: `
			CREATE PROCEDURE loadGuildData()
			BEGIN
				SELECT * FROM guildlist;
			END
		`,
	},
	{
		Name: "saveGuildData",
		SQL: `
			CREATE PROCEDURE saveGuildData (IN guild_id BIGINT,
											IN server_id INTEGER,
											IN guild_name VARCHAR(64),
											IN level INTEGER,
											IN leader_id BIGINT,
											IN leader_name VARCHAR(64),
											IN leader_boundary_lv INTEGER,
											IN apply_level INTEGER,
											IN apply_power INTEGER,
											IN approval_mode INTEGER,
											IN notice VARCHAR(128),
											IN money INTEGER,
											IN binary_data blob,
											IN creat_time INTEGER
											)
			BEGIN
				REPLACE INTO guildlist(guild_id, srv_id, name, level, leader_id, leader_name, leader_boundary_lv, apply_level, apply_power, approval_mode, notice, money, binary_data, creat_time) VALUE (guild_id, server_id, guild_name, level, leader_id, leader_name, leader_boundary_lv, apply_level, apply_power, approval_mode, notice, money, binary_data, creat_time);
			END
		`,
	},
	{
		Name: "loadGuildMemberData",
		SQL: `
			CREATE PROCEDURE loadGuildMemberData (IN server_id INTEGER, IN guildId BIGINT)
			BEGIN
				SELECT * FROM guildmember WHERE srv_id = server_id and guild_id = guildId;
			END
		`,
	},
	{
		Name: "saveGuildMemberData",
		SQL: `
			CREATE PROCEDURE saveGuildMemberData (IN guildId BIGINT, IN server_id INTEGER, IN actor_id BIGINT, IN position INTEGER, IN donate INTEGER, IN join_time INTEGER)
			BEGIN
				REPLACE INTO guildmember(guild_id, srv_id, actor_id, position, donate, join_time) VALUES (guildId, server_id, actor_id, position, donate, join_time);
			END
		`,
	},
	{
		Name: "deleteGuild",
		SQL: `
			CREATE PROCEDURE deleteGuild (IN Id BIGINT)
			BEGIN
				DELETE FROM guildlist where guild_id=Id;
				DELETE FROM guildmember where guild_id=Id;
				DELETE FROM guilddepot WHERE guild_id=Id;
			END
		`,
	},
	{
		Name: "deleteGuildMember",
		SQL: `
			CREATE PROCEDURE deleteGuildMember (IN Id BIGINT, IN actorId BIGINT)
			BEGIN
				DELETE FROM guildmember where guild_id=Id and actor_id = actorId;
			END
		`,
	},
	{
		Name: "loadFriends",
		SQL: `
			CREATE PROCEDURE loadFriends(IN actorId BIGINT)
		BEGIN
			SELECT a.friend_id, a.f_type,intimacy from friends a, actors b
			WHERE (b.status & 2)=2 and a.actor_id = actorId and a.friend_id = b.actor_id;
		END
		`,
	},
	{
		Name: "updatefriends",
		SQL: `
		CREATE PROCEDURE updateFriends (IN actorId bigint, IN friendId bigint, IN opCode integer, IN fType integer)
		BEGIN
			DECLARE aid bigint;
			DECLARE cur integer;
			if opCode = 0 then
				SELECT f_type into cur FROM friends where actor_id = actorId and friend_id = friendId and (f_type & (1 <<fType)) > 0;
				if cur = (1 << fType) then
					DELETE FROM friends where actor_id = actorId and friend_id = friendId;
				else
					update friends set f_type = cur & ~(1 << fType) where actor_id = actorId and friend_id = friendId;
					if (fType & 2) > 0 then
						update friends set intimacy = 0 where actor_id = actorId and friend_id = friendId;
					end if;
				end if;
			end if;
			if opCode = 1 then
				select actor_id into aid from friends where actor_id=actorId and friend_id = friendId;
				if aid is null then
					insert into friends(actor_id,friend_id,f_type) values(actorId, friendId, (1 << fType));
				else
					update friends set f_type = (f_type | (1 << fType)) where actor_id = actorId and friend_id = friendId;
				end if;
			end if;
		END
	`,
	},

	{
		Name: "updatefriendsintimacy",
		SQL: `
		CREATE PROCEDURE updatefriendsintimacy (IN actorId bigint, IN friendId bigint, IN newIntimacy integer)
		BEGIN
			update friends set intimacy = newIntimacy where actor_id = actorId and friend_id = friendId and (f_type & 2)>0;
		END
	`,
	},
	{
		Name: "addActorMsg",
		SQL: `
		CREATE PROCEDURE addActorMsg (IN nActorId bigint, IN nMsgType integer, IN sMsg tinyblob)
		BEGIN
			INSERT INTO actormsg(actor_id,msg_type,msg) values (nActorId,nMsgType,sMsg);
			SELECT LAST_INSERT_ID() as msgId;
		END
	`,
	},
	{
		Name: "deleteActorMsg",
		SQL: `
		CREATE PROCEDURE deleteActorMsg (IN nActorId bigint, IN nMsgId bigint)
		BEGIN
			DELETE FROM actormsg WHERE msg_id=nMsgId and actor_id=nActorId;
		END
	`,
	},
	{
		Name: "loadActorMsgList",
		SQL: `
		CREATE PROCEDURE loadActorMsgList (in nActorId bigint, in nMsgId bigint)
		BEGIN
			IF nMsgId = 0 THEN
				SELECT msg_id,msg_type,msg from actormsg WHERE actor_id=nActorId;
			ELSE
				SELECT msg_id,msg_type,msg from actormsg WHERE actor_id=nActorId and msg_id=nMsgId;
			END IF;
		END
	`,
	},
	{
		Name: "loadRankList",
		SQL: `
		CREATE PROCEDURE loadRankList (in nRankType integer)
		BEGIN
			IF nRankType = 0 THEN
				SELECT rank_type, rank_data from rank;
			ELSE
				SELECT rank_type, rank_data from rank WHERE rank_type = nRankType;
			END IF;
		END
		`,
	},
	{
		Name: "checkPlayerStatus",
		SQL: `
		CREATE PROCEDURE checkPlayerStatus (in nPlayerId bigint)
		BEGIN
			select status from actors where actor_id=nPlayerId;
		END
		`,
	},
	{
		Name: "updateRank",
		SQL: `
			create procedure updateRank(in nRankType integer, IN bRankData mediumblob)
			begin
				DELETE FROM rank WHERE rank_type=nRankType;
				insert into rank(rank_type, rank_data) values(nRankType, bRankData);
			end
		`,
	},
	{
		Name: "loadplayerbasic",
		SQL: `
			create procedure loadplayerbasic(in nactorid bigint)
			begin
				select
					actors.actor_id as actor_id,
					sex,
					job,
					circle,
					level,
					actor_name,
					vip,
					fight_value,
					last_logout_time

				from actors where actors.actor_id = nactorid;
			end
		`,
	},
	{
		Name: "updateserverinfo",
		SQL: `
			create procedure updateserverinfo(in serverId int unsigned, in create_flag int unsigned)
			begin
				REPLACE INTO serverinfo(server_id, forbid_create_flag) VALUES(serverId, create_flag) ;
			end
		`,
	},
	{
		Name: "loadGlobalVar",
		SQL: `
		CREATE PROCEDURE loadGlobalVar(in serverId int unsigned)
		BEGIN
			SELECT server_id, binary_data FROM globalvar WHERE server_id = serverId;
		END
	`,
	},
	{
		Name: "loadAllGlobalVar",
		SQL: `
		CREATE PROCEDURE loadAllGlobalVar()
		BEGIN
			SELECT server_id, binary_data FROM globalvar;
		END
	`,
	},
	{
		Name: "saveGlobalVar",
		SQL: `
			create procedure saveglobalvar(in serverId int unsigned, in bData mediumblob)
			begin
				REPLACE INTO globalvar(server_id, binary_data) VALUES(serverId, bData) ;
			end
		`,
	},
	{
		Name: "delGlobalVar",
		SQL: `
			create procedure delGlobalVar(in serverId int unsigned)
			begin
				DELETE FROM globalvar WHERE server_id = serverId ;
			end
		`,
	},
	{
		Name: "getFriendStatus",
		SQL: `
			CREATE PROCEDURE getFriendStatus ( IN nactorid BIGINT, IN friendid BIGINT) BEGIN
				SELECT count(*) as isblack FROM friends WHERE actor_id = nactorid AND friend_id = friendid AND (f_type & 8) > 0;
			END
		`,
	},
}
