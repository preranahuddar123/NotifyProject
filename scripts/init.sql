CREATE TABLE IF NOT EXISTS leadDetails (
  lead_identifier VARCHAR(100) PRIMARY KEY,
  lead_name       VARCHAR(100) NOT NULL,
  lead_type       ENUM(
                    'SALES_POOL',
                    'EXTERNAL_LEAD',
                    'GOOGLE_ADS',
                    'META_ADS',
                    'ADD_LEAD',
                    'IVR_CALL',
                    'WEBSITE_LEAD',
                    'WALK_IN_LEAD',
                    'WHATSAPP'
                  ) NOT NULL,
  assigned_to     VARCHAR(100),
  created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS meeting_details (
  meeting_id       INT AUTO_INCREMENT PRIMARY KEY,
  lead_identifier  VARCHAR(100) NOT NULL,
  lead_name        VARCHAR(100) NOT NULL,
  sub_stage        ENUM('SCHEDULED', 'RESCHEDULED', 'CANCELLED') NOT NULL,
  title            VARCHAR(150),
  meeting_date     DATE,
  slot             VARCHAR(50),
  meeting_type     ENUM('VIRTUAL_MEETING', 'SHOWROOM_VISIT', 'SITE_VISIT') NOT NULL,
  milestone        ENUM('CONNECTION', 'EXPERIENCE_AND_DESIGN'),
  created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS successful (
  lead_identifier      VARCHAR(100) NOT NULL,
  next_action          VARCHAR(100),
  quote_link_generated TINYINT(1) DEFAULT 0,
  created_at           TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS booking (
  booking_id        INT AUTO_INCREMENT PRIMARY KEY,
  lead_identifier   VARCHAR(100) NOT NULL,
  payment_type      ENUM('TOKEN', 'BOOKING FUll_10') NOT NULL,
  paid_amount       DECIMAL(10, 2) NOT NULL,
  Remaining_amount  DECIMAL(10, 2) NOT NULL,
  payment_date      DATETIME NOT NULL,
  payment_status    ENUM('PENDING', 'SUCCESS', 'FAILED') DEFAULT 'PENDING',
  remarks           TEXT,
  created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS notification (
  notification_id  INT AUTO_INCREMENT PRIMARY KEY,
  lead_identifier  VARCHAR(100) NOT NULL,
  source_module    ENUM('CRM') DEFAULT 'CRM',
  event_type       VARCHAR(50) NOT NULL,
  ref_table        VARCHAR(50) NOT NULL,
  ref_id           INT NOT NULL,
  idempotency_key  VARCHAR(255) NOT NULL,
  created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_notification_idempotency_key (idempotency_key)
);

CREATE TABLE IF NOT EXISTS notification_recipients (
  recipient_id     INT AUTO_INCREMENT PRIMARY KEY,
  notification_id  INT NOT NULL,
  user_id          INT NOT NULL,
  recipient_type   VARCHAR(50) NOT NULL,
  delivery_status  VARCHAR(20) DEFAULT 'PENDING',
  seen_at          TIMESTAMP NULL DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS booking (
    booking_id        INT AUTO_INCREMENT PRIMARY KEY,
    lead_identifier   VARCHAR(100) NOT NULL,
    lead_name         VARCHAR(255) NOT NULL,
    payment_type      ENUM('TOKEN', 'BOOKING') NOT NULL,
    paid_amount       DECIMAL(10,2) NOT NULL,
    remaining_amount  DECIMAL(10,2) NOT NULL,
    payment_date      DATETIME NOT NULL,
    payment_status    ENUM('PENDING', 'SUCCESS', 'FAILED') DEFAULT 'PENDING',
    remarks           TEXT,
    created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);