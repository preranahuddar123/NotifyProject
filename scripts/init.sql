CREATE TABLE IF NOT EXISTS leadDetails (
  lead_id   INT PRIMARY KEY,
  lead_name VARCHAR(255) NOT NULL
);

CREATE TABLE IF NOT EXISTS meeting_details (
  meeting_id     INT AUTO_INCREMENT PRIMARY KEY,
  lead_id        INT NOT NULL,
  lead_name      VARCHAR(255) NOT NULL,
  milestone      VARCHAR(50) NOT NULL,
  reason         TEXT,
  title          VARCHAR(255),
  description    TEXT,
  meeting_date   VARCHAR(64),
  slot           VARCHAR(64),
  meeting_type   VARCHAR(64),
  stage          VARCHAR(64),
  event_datetime DATETIME NULL,
  created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS successful (
  lead_id               INT PRIMARY KEY,
  description           TEXT,
  stage                 VARCHAR(64),
  success_datetime      DATETIME NULL,
  next_action           TEXT,
  quote_link_generated  TINYINT(1) DEFAULT 0,
  created_at            TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS meeting_scheduled (
  id           INT AUTO_INCREMENT PRIMARY KEY,
  lead_id      INT NOT NULL,
  lead_name    VARCHAR(255) NOT NULL,
  title        VARCHAR(255),
  description  TEXT,
  meeting_date VARCHAR(64),
  slot         VARCHAR(64),
  meeting_type VARCHAR(64),
  stage        VARCHAR(64),
  created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS meeting_rescheduled (
  id           INT AUTO_INCREMENT PRIMARY KEY,
  lead_id      INT NOT NULL,
  lead_name    VARCHAR(255) NOT NULL,
  title        VARCHAR(255),
  description  TEXT,
  meeting_date VARCHAR(64),
  slot         VARCHAR(64),
  meeting_type VARCHAR(64),
  stage        VARCHAR(64),
  reason       TEXT,
  created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
