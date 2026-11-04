-- 完整店铺示例：由部署者显式执行，不在服务启动时自动迁移。
CREATE TABLE IF NOT EXISTS shops (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name VARCHAR(50) NOT NULL,
    status VARCHAR(10) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_shops_status_id (status, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
