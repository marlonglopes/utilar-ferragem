-- ============================================================================
-- Configuração da loja (config editável pelo dono, sem deploy)
-- ----------------------------------------------------------------------------
-- POR QUÊ: hoje o único jeito de mudar o aviso da vitrine (promoção, horário de
-- feriado, "loja fechada dia X") é editar código e subir. Isso põe o dono na
-- dependência de um deploy pra uma mensagem que muda toda semana. Esta tabela
-- guarda a config que a vitrine lê publicamente e que o admin edita.
--
-- SINGLETON: é UMA loja, então uma linha só. `id` é uma constante travada em 1
-- por CHECK — nunca há uma segunda linha, e o GET/PUT sempre miram id=1. É mais
-- simples e mais seguro que "pegue a linha mais recente" (que abriria a porta pra
-- duas configs divergentes e um bug de "qual vale?").
--
-- NÃO guarda dado cadastral legal (CNPJ, razão social, endereço fiscal): isso
-- mora em app/src/lib/company.ts e depende de revisão jurídica (Decreto
-- 7.962/2013) — não é campo pra editar solto num painel. Aqui só o que é
-- operação do dia a dia: o aviso da vitrine.
--
-- Reversível.
-- ============================================================================
CREATE TABLE store_settings (
    id                   INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    announcement_enabled BOOLEAN NOT NULL DEFAULT false,
    announcement_message TEXT    NOT NULL DEFAULT '',
    -- Tom do aviso: define a cor/ícone na vitrine. Travado no banco pra um valor
    -- inesperado não quebrar o render nem virar vetor de injeção de classe CSS.
    announcement_level   TEXT    NOT NULL DEFAULT 'info'
                         CHECK (announcement_level IN ('info', 'warning', 'success')),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by           TEXT NOT NULL DEFAULT ''
);

-- Semeia a linha singleton já desligada: o GET público nunca precisa lidar com
-- "linha ausente", e o aviso nasce invisível (nada aparece até o dono ligar).
INSERT INTO store_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
