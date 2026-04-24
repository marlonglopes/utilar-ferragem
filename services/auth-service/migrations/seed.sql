-- ============================================================================
-- Seed do auth-service
-- ----------------------------------------------------------------------------
-- 20 users de teste:
--   * test1@utilar.com.br   customer   ← use para a maioria dos fluxos
--   * test2..test17@...     customer
--   * seller1@utilar.com.br seller
--   * seller2@utilar.com.br seller
--   * admin@utilar.com.br   admin
--
-- Todos com senha "utilar123" (hash argon2id abaixo, gerado via `go run ./cmd/hash utilar123`).
-- Como argon2id encoded inclui o salt, o mesmo hash pode ser replicado em todos os seeds
-- determinísticos — qualquer dev que rode o seed autenticará com `utilar123`.
-- Emails confirmados de forma já validada (email_verified = true) para poupar o flow de verify em dev.
-- ============================================================================

BEGIN;

TRUNCATE TABLE refresh_tokens, password_reset_tokens, email_verification_tokens, addresses, users RESTART IDENTITY CASCADE;

-- ---------------------------------------------------------------------------
-- 20 users com senha = "utilar123"
-- ---------------------------------------------------------------------------
INSERT INTO users (id, email, password_hash, name, cpf, phone, role, email_verified) VALUES
    ('00000000-0000-4000-8000-000000000001', 'test1@utilar.com.br',   '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Ana Silva',           '12345678901', '11988887777', 'customer', true),
    ('00000000-0000-4000-8000-000000000002', 'test2@utilar.com.br',   '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Bruno Ferreira',     '23456789012', '11999996666', 'customer', true),
    ('00000000-0000-4000-8000-000000000003', 'test3@utilar.com.br',   '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Carla Oliveira',     '34567890123', NULL,          'customer', true),
    ('00000000-0000-4000-8000-000000000004', 'test4@utilar.com.br',   '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Daniel Santos',      '45678901234', '11988881111', 'customer', true),
    ('00000000-0000-4000-8000-000000000005', 'test5@utilar.com.br',   '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Eduarda Lima',       '56789012345', '11988882222', 'customer', true),
    ('00000000-0000-4000-8000-000000000006', 'test6@utilar.com.br',   '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Fábio Almeida',      '67890123456', NULL,          'customer', true),
    ('00000000-0000-4000-8000-000000000007', 'test7@utilar.com.br',   '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Gabriela Rocha',     '78901234567', '11988883333', 'customer', true),
    ('00000000-0000-4000-8000-000000000008', 'test8@utilar.com.br',   '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Henrique Costa',     '89012345678', NULL,          'customer', true),
    ('00000000-0000-4000-8000-000000000009', 'test9@utilar.com.br',   '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Isabela Martins',    '90123456789', '11988884444', 'customer', true),
    ('00000000-0000-4000-8000-000000000010', 'test10@utilar.com.br',  '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'João Pereira',       '01234567890', '11988885555', 'customer', true),
    ('00000000-0000-4000-8000-000000000011', 'test11@utilar.com.br',  '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Karen Nunes',        '11122233344', NULL,          'customer', false),
    ('00000000-0000-4000-8000-000000000012', 'test12@utilar.com.br',  '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Lucas Carvalho',     '22233344455', NULL,          'customer', true),
    ('00000000-0000-4000-8000-000000000013', 'test13@utilar.com.br',  '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Marina Souza',       '33344455566', '11988886666', 'customer', true),
    ('00000000-0000-4000-8000-000000000014', 'test14@utilar.com.br',  '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Nicolas Dias',       '44455566677', NULL,          'customer', true),
    ('00000000-0000-4000-8000-000000000015', 'test15@utilar.com.br',  '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Olívia Fernandes',   '55566677788', '11988887777', 'customer', true),
    ('00000000-0000-4000-8000-000000000016', 'test16@utilar.com.br',  '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Pedro Barros',       '66677788899', NULL,          'customer', true),
    ('00000000-0000-4000-8000-000000000017', 'test17@utilar.com.br',  '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Rafaela Azevedo',    '77788899900', '11988888888', 'customer', true),
    ('00000000-0000-4000-8000-000000000018', 'seller1@utilar.com.br', '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Seller Ferragem Silva', '88899900011', '1133334444', 'seller',   true),
    ('00000000-0000-4000-8000-000000000019', 'seller2@utilar.com.br', '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Seller Pro Tools BR',   '99900011122', '1133335555', 'seller',   true),
    ('00000000-0000-4000-8000-000000000020', 'admin@utilar.com.br',   '$argon2id$v=19$m=19456,t=2,p=1$ai8tdn7jzEPMAiBcC34pVQ$Cl3ybZ+GV09T3tCPEnNorHj3Cs+FRGgkrRAyPKR4jqY', 'Marlon (admin)',     NULL,          NULL,         'admin',    true);

-- ---------------------------------------------------------------------------
-- Endereços (~30 linhas; nem todo user tem endereço, alguns têm 2)
-- ---------------------------------------------------------------------------
INSERT INTO addresses (user_id, label, street, number, neighborhood, city, state, cep, is_default)
SELECT
    u.id,
    'Principal',
    'Rua das Ferragens',
    (100 + ROW_NUMBER() OVER (ORDER BY u.email))::text,
    'Centro',
    'São Paulo',
    'SP',
    lpad((1000 + ROW_NUMBER() OVER (ORDER BY u.email))::text, 5, '0') || '-000',
    true
FROM users u
WHERE u.email LIKE 'test%' OR u.role = 'seller';

-- 10 endereços secundários ("Trabalho")
INSERT INTO addresses (user_id, label, street, number, complement, neighborhood, city, state, cep, is_default)
SELECT u.id, 'Trabalho', 'Av Paulista',
    (1000 + ROW_NUMBER() OVER (ORDER BY u.email))::text,
    'Sala 12', 'Bela Vista', 'São Paulo', 'SP', '01310-100', false
FROM users u
WHERE u.email IN ('test1@utilar.com.br','test3@utilar.com.br','test5@utilar.com.br','test7@utilar.com.br','test9@utilar.com.br',
                  'test11@utilar.com.br','test13@utilar.com.br','test15@utilar.com.br','seller1@utilar.com.br','seller2@utilar.com.br');

COMMIT;

SELECT 'users'                     AS table_name, count(*) AS rows FROM users
UNION ALL SELECT 'addresses',       count(*) FROM addresses
UNION ALL SELECT 'refresh_tokens',  count(*) FROM refresh_tokens
UNION ALL SELECT 'email_verification_tokens', count(*) FROM email_verification_tokens
UNION ALL SELECT 'password_reset_tokens',     count(*) FROM password_reset_tokens;
