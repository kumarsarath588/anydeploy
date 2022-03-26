CREATE TABLE IF NOT EXISTS `anydeploy`.`deployments` (
    `uuid` VARCHAR(255) NOT NULL,
    `type` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `state` VARCHAR(255) NOT NULL,
    `payload` TEXT NOT NULL,
    PRIMARY KEY (`uuid`)
);