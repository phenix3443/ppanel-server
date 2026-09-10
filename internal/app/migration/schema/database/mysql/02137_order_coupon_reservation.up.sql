ALTER TABLE `order`
    ADD COLUMN `coupon_reserved` TINYINT(1) NOT NULL DEFAULT 0 COMMENT 'Coupon usage reserved while order is pending' AFTER `coupon_discount`;
