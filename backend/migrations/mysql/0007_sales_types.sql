-- Shared sales classifications. Display names and availability are maintained
-- through Masters; supported accounting classifications retain stable identities.
CREATE TABLE sales_types (
 id BINARY(16) NOT NULL PRIMARY KEY,
 code VARCHAR(100) NOT NULL UNIQUE,
 name VARCHAR(191) NOT NULL,
 sales_class VARCHAR(32) NOT NULL UNIQUE,
 active BOOLEAN NOT NULL DEFAULT TRUE,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 deleted_at DATETIME(6) NULL,
 CONSTRAINT sales_types_class CHECK (sales_class IN ('international','domestic','miscellaneous'))
) ENGINE=InnoDB;
INSERT INTO sales_types(id,code,name,sales_class) VALUES
 (UNHEX('00000000000040008000000000003001'),'INTL','International','international'),
 (UNHEX('00000000000040008000000000003002'),'DOM','Domestic','domestic'),
 (UNHEX('00000000000040008000000000003003'),'MISC','Miscellaneous','miscellaneous');
