-- Apply once after 0004_employee_access.sql.
-- Existing accounts retain username-based sign-in.
ALTER TABLE users
    ADD COLUMN login_method ENUM('username', 'email') NOT NULL DEFAULT 'username' AFTER username;
