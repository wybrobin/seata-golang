-- -------------------------------- The script used when storeMode is 'db' --------------------------------

CREATE database if NOT EXISTS `seata` default character set utf8mb4 collate utf8mb4_unicode_ci;
USE `seata`;

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;
-- the table to store GlobalSession data
--对应pb中的 message GlobalSession，多了gmt_create和gmt_modified
CREATE TABLE IF NOT EXISTS `global_table`
(
  `addressing` varchar(128) NOT NULL,
  `xid` varchar(128) NOT NULL,	--格式：{addressing}:{transaction_id}，addressing就是sample里tm和rm配置里的名字
  `transaction_id` bigint DEFAULT NULL,	--低12位保存自增值，然后中间41位保存加工过的毫秒时间戳，剩下的高位保存workerID，是TC启动的命令行参数serverNode值
  `transaction_name` varchar(128) DEFAULT NULL,
  `timeout` int DEFAULT NULL,
  `begin_time` bigint DEFAULT NULL,
  `status` tinyint NOT NULL,
  `active` bit(1) NOT NULL,
  `gmt_create` datetime DEFAULT NULL,
  `gmt_modified` datetime DEFAULT NULL,
  PRIMARY KEY (`xid`),
  KEY `idx_gmt_modified_status` (`gmt_modified`,`status`),
  KEY `idx_transaction_id` (`transaction_id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

-- the table to store BranchSession data
--与global_table具有相同xid的branch_id，都属于global_table的xid那条记录
--一个global_table的xid对应多个具有相同xid的branch_table的branch_id
CREATE TABLE IF NOT EXISTS `branch_table`
(
  `addressing` varchar(128) NOT NULL,
  `xid` varchar(128) NOT NULL,
  `branch_id` bigint NOT NULL,	--与transaction_id生成方式一样
  `transaction_id` bigint DEFAULT NULL,
  `resource_id` varchar(256) DEFAULT NULL,	--就是DBName
  `lock_key` VARCHAR(1000),	--用;隔开的。每个:前代表tableName，后代表mergedPKs。mergedPKs又是用,隔开每个pk
  `branch_type` varchar(8) DEFAULT NULL, -- 0at，1tcc，2saga,3xa
  `status` tinyint DEFAULT NULL,
  `application_data` varchar(2000) DEFAULT NULL,
  `gmt_create` datetime(6) DEFAULT NULL,
  `gmt_modified` datetime(6) DEFAULT NULL,
  PRIMARY KEY (`branch_id`),
  KEY `idx_xid` (`xid`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;

-- the table to store lock data
--branch_table里的一个resource_id和lock_key用:拆分后的tableName，下面又用,拆开一些pk，对应lock_table的一个row_key
--也就是branch_table的resource_id和lock_key合起来保存了多个lock_table的主键row_key
CREATE TABLE IF NOT EXISTS `lock_table`
(
    `row_key`        VARCHAR(256) NOT NULL,
    `xid`            VARCHAR(128) NOT NULL,
    `transaction_id` BIGINT,
    `branch_id`      BIGINT       NOT NULL,
    `resource_id`    VARCHAR(256),
    `table_name`     VARCHAR(64),
    `pk`             VARCHAR(36),
    `gmt_create`     DATETIME,
    `gmt_modified`   DATETIME,
    PRIMARY KEY (`row_key`),
    KEY `idx_branch_id` (`branch_id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8;
