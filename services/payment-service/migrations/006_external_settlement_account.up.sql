-- ============================================================================
-- Liquidação externa: conta de caixa do adquirente próprio (maquininha da loja)
-- ----------------------------------------------------------------------------
-- A venda de balcão paga na maquininha da loja não passa pela Appmax. O
-- dinheiro entra por um adquirente próprio e é depositado direto na conta da
-- empresa, sem nunca existir no nosso PSP.
--
-- Até aqui esse dinheiro era lançado como se fosse `card` capturado pelo PSP:
--   * a 1.1.1 (caixa em trânsito no PSP) acumulava saldo que a Appmax nunca
--     teve — e a conciliação (reconcile.go) acusaria divergência para sempre;
--   * o relatório por método de pagamento somava maquininha com cartão online.
--
-- A conta 1.1.3 separa as duas origens de dinheiro. Ela NÃO entra na
-- conciliação com o PSP, por construção: reconcile.go só percorre `payments`
-- com psp_payment_id preenchido, e liquidação externa não cria linha em
-- `payments` nenhuma.
--
-- A contrapartida continua sendo a 3.1.1 (receita bruta): faturamento é
-- faturamento, independente de por onde o dinheiro entrou. O que distingue é a
-- conta de ativo, o `kind` = 'external_sale' e o `payment_method` = 'external'
-- nas partidas.
-- ============================================================================

INSERT INTO ledger_accounts (code, name, type, normal_side, description) VALUES
    ('1.1.3', 'Caixa em trânsito no adquirente externo', 'asset', 'debit',
     'Dinheiro capturado pela maquininha da loja (adquirente próprio, fora da Appmax), '
     'até o depósito na conta da empresa. Fora do escopo da conciliação com o PSP.');
