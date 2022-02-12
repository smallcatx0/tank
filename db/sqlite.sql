CREATE TABLE `users` (
  `id` int(11) NOT NULL,
  `user` varchar(32) DEFAULT NULL COMMENT '账号',
  `pass` varchar(64) DEFAULT NULL COMMENT '密码',
  `nickname` varchar(32) DEFAULT NULL COMMENT '昵称',
  `truename` varchar(32) DEFAULT NULL COMMENT '真实姓名',
  `phone` char(20) DEFAULT NULL COMMENT '手机号',
  `email` varchar(64) DEFAULT NULL COMMENT '电子邮箱',
  `status` tinyint(4) DEFAULT NULL COMMENT '状态',
  `created_at` timestamp NULL DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT NULL,
  `deleted_at` timestamp NULL DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;