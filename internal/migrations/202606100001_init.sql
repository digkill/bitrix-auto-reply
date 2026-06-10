-- +goose Up
CREATE TABLE rules (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    keywords TEXT NOT NULL,
    action_type VARCHAR(50) NOT NULL,
    response_text TEXT NULL,
    file_url TEXT NULL,
    api_url TEXT NULL,
    is_active TINYINT(1) NOT NULL DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE processed_messages (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    bitrix_message_id BIGINT NOT NULL UNIQUE,
    dialog_id VARCHAR(100) NOT NULL,
    author_id BIGINT NOT NULL,
    message_text TEXT,
    answer_text TEXT,
    rule_id BIGINT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_dialog_id (dialog_id),
    INDEX idx_author_id (author_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE dialog_locks (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    dialog_id VARCHAR(100) NOT NULL UNIQUE,
    last_answer_at TIMESTAMP NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO rules 
(name, keywords, action_type, response_text, is_active)
VALUES
('Цена', 'цена,прайс,стоимость,сколько стоит', 'text', 'Привет! По цене сейчас уточню и напишу.', 1),
('Оплата', 'оплата,оплатить,счет,счёт', 'text', 'Привет! По оплате сейчас посмотрю.', 1),
('Срочно', 'срочно,важно,быстро', 'text', 'Привет! Увидел сообщение, сейчас посмотрю.', 1);

-- +goose Down
DROP TABLE IF EXISTS dialog_locks;
DROP TABLE IF EXISTS processed_messages;
DROP TABLE IF EXISTS rules;