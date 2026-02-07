#主服：[{{.Master}}]
#从服列表：{{.Slave}}

{{- $dbprefix := concat .Pf "_actor_"}}
{{$masterprefix := concat $dbprefix .Master}}

#----------------------------------------------------------------#
#从{{.Pf}}_actor_{{.Master}}清除本服过期邮件
delete from actormail where status = 3;

#从{{.Pf}}_actor_{{.Master}}清除本服全服邮件
delete from actorservermail;
delete from server_mail;
#从{{.Pf}}_actor_{{.Master}}清除本服旧的冲榜数据
delete from rush_rank;
#从{{.Pf}}_actor_{{.Master}}清除本服旧的机器人玩家镜像
delete from actor_robot;
#----------------------------------------------------------------#

{{- range $index, $sId := .Slave -}}

{{$prefix := concat $dbprefix $sId}}

#----------------------------------------------------------------#
#删除itempool旧索引
update {{$prefix}}.itempool set id = id|{{$sId}} << 37 where id >> 37 = 0;

#从{{$prefix}}清除本服过期邮件
delete from {{$prefix}}.actormail where status = 3;

#从{{$prefix}}清除全服邮件邮件
delete from {{$prefix}}.actorservermail;
delete from {{$prefix}}.server_mail;
#----------------------------------------------------------------#

#----------------------------------------------------------------#
#处理角色重名
drop table if exists tmp_table;

create temporary table tmp_table select actor_name, actor_id from actors;

alter table tmp_table add index tmp_table(actor_name);

update {{$prefix}}.actors set actor_name=CONCAT('s{{$sId}}.', actor_name), status = (status | 8)
	where (actor_name in (select actor_name from tmp_table where {{$prefix}}.actors.actor_id <> tmp_table.actor_id));

#从{{$prefix}}导入角色数据
insert into {{$masterprefix}}.actors (select * from {{$prefix}}.actors);
#----------------------------------------------------------------#

#----------------------------------------------------------------#
#处理行会重名
drop table if exists tmp_table;

create temporary table tmp_table select name, guild_id, leader_id from guildlist;

alter table tmp_table add index tmp_table(name);

update {{$prefix}}.guildlist set name=CONCAT('s{{$sId}}.', name)
	where (name in (select name from tmp_table where {{$prefix}}.guildlist.guild_id <> tmp_table.guild_id));

#从{{$prefix}}导入guildlist数据
insert into guildlist (select * from {{$prefix}}.guildlist);

#把帮会名字包含"s."的leader_id筛选到一张表中
drop table if exists tmp_table;

create temporary table tmp_table (`leader_id` bigint not null primary key);

insert into tmp_table (select leader_id from guildlist where name like "%s{{$sId}}.%" and leader_id <> 0);

update actors set status = (status | 16) where actor_id in (select leader_id from tmp_table);
#----------------------------------------------------------------#

#----------------------------------------------------------------#
#从{{$prefix}}导入account数据
insert into account select * from {{$prefix}}.account where {{$prefix}}.account.user_id not in (select user_id from account);

#从{{$prefix}}导入actor_binary_data数据
insert into actor_binary_data (select * from {{$prefix}}.actor_binary_data);

#从{{$prefix}}导入actorinfo数据
#insert into actorinfo (select * from {{$prefix}}.actorinfo);

#从{{$prefix}}导入actormail数据
insert into actormail (select * from {{$prefix}}.actormail);

#从{{$prefix}}导入actormsg数据
insert into actormsg(actor_id,msg_type,msg) (select actor_id,msg_type,msg from {{$prefix}}.actormsg);

#从{{$prefix}}导入skill数据
insert into skill (select * from {{$prefix}}.skill);

#从{{$prefix}}导入friends数据
insert into friends (select * from {{$prefix}}.friends);

#从{{$prefix}}导入guildmember数据
insert into guildmember (select * from {{$prefix}}.guildmember);

#从{{$prefix}}guilddepot
insert into guilddepot (select * from {{$prefix}}.guilddepot);

#从{{$prefix}}导入itempool数据
insert into itempool (select * from {{$prefix}}.itempool);

#从{{$prefix}}导入offlinedata数据
insert into offlinedata (select * from {{$prefix}}.offlinedata);

#从{{$prefix}}导入rank数据
delete from rank where rank_type=14;
insert into rank (select * from {{$prefix}}.rank where rank_type<>14);

#{{$prefix}}移除旧的冲榜数据
#delete from {{$prefix}}.rush_rank where rank_type>0

#从{{$prefix}}导入ask_help数据
insert into ask_help (select * from {{$prefix}}.ask_help);

#从{{$prefix}}online_data
insert into online_data (select * from {{$prefix}}.online_data);

#从{{$prefix}}导入globalVar数据
insert into globalvar (select * from {{$prefix}}.globalvar);

#从{{$prefix}}导入actor_series数据
insert into actor_series (`server_id`,`series`)
select `server_id`,`series` from {{$prefix}}.actor_series;

#从{{$prefix}}导入actor_statics数据
update {{$prefix}}.actor_statics set id = id|{{$sId}} << 17 where id >> 17 = 0;
insert into actor_statics (select * from {{$prefix}}.actor_statics);
#----------------------------------------------------------------#
{{- end}}

#----------------------------------------------------------------#
#删除数据

drop table if exists tmp_table;

create table tmp_table(`actor_id` bigint not null primary key);

insert into tmp_table (select actor_id from actors where
(vip=0 and circle < 4 and lastlogintime<subdate(now(),interval 14 day))
or (vip=0 and circle >= 4 and lastlogintime<=subdate(now(),interval 30 day))
or (1 <= vip and vip < 3 and lastlogintime<=subdate(now(),interval 60 day))
or (3 <= vip and vip < 5 and lastlogintime<=subdate(now(),interval 90 day))
or (5 <= vip and vip < 8 and lastlogintime<=subdate(now(),interval 180 day))
or (8 <= vip and vip < 10 and lastlogintime<=subdate(now(),interval 360 day)));

delete from actor_binary_data where actor_id in (select actor_id from tmp_table);

#delete from actorinfo where actor_id in (select actor_id from tmp_table);

delete from actormail where actor_id in (select actor_id from tmp_table);

delete from actormsg where actor_id in (select actor_id from tmp_table);

delete from actors where actor_id in (select actor_id from tmp_table);

delete from friends where actor_id in (select actor_id from tmp_table);

delete from guildmember where (actor_id & (1 << 48)) > 0;

delete from guildmember where actor_id in (select actor_id from tmp_table);

update guildlist set leader_id = 0, leader_name="" where leader_id in (select actor_id from tmp_table);

delete from itempool where actor_id in (select actor_id from tmp_table);

delete from offlinedata where actor_id in (select actor_id from tmp_table);

delete from skill where actor_id in (select actor_id from tmp_table);

# 删除求助记录
delete from ask_help where player_id in (select actor_id from tmp_table);

delete from online_data where player_id in (select actor_id from tmp_table);

#删除没有成员的行会
delete from guildlist where guild_id not in (select guild_id from guildmember);

delete from yy_cmd_setting where status = 2;
#----------------------------------------------------------------#
