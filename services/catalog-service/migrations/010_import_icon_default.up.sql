-- ============================================================================
-- products.icon: NOT NULL sem DEFAULT bloqueava toda a ingestão
-- ----------------------------------------------------------------------------
-- `icon` é um glyph de vitrine — decisão de MERCHANDISING, não dado de
-- fornecedor. Nenhuma planilha traz essa coluna, e ela não está (nem deve
-- estar) na whitelist de campos mapeáveis do perfil de importação.
--
-- O resultado era o pior tipo de falha: o INSERT do commit omitia `icon`, o
-- Postgres não tinha DEFAULT pra suprir, e a constraint NOT NULL derrubava
-- TODAS as linhas. Como o commit usa uma transação por linha (para que linha
-- ruim não aborte o lote), cada falha virava `res.Failed++` em vez de erro de
-- lote — a importação respondia 200 OK tendo gravado zero produtos. "Subi e
-- não aconteceu nada", sem nada vermelho para investigar.
--
-- ⚠️ Lembrete do porquê de DEFAULT resolver aqui: no Postgres o DEFAULT só é
-- aplicado quando a coluna é OMITIDA do INSERT. Passar NULL explícito continua
-- violando o NOT NULL. Por isso a correção é o DEFAULT + o committer seguir
-- omitindo a coluna, e não passar NULL "deixando o banco resolver".
--
-- O glyph escolhido é neutro e genérico: o produto importado entra como
-- rascunho de qualquer forma, e quem publica escolhe o ícone de verdade.
-- ============================================================================

ALTER TABLE products ALTER COLUMN icon SET DEFAULT '📦';

-- Linhas legadas com string vazia ficam com o mesmo placeholder, para que a
-- vitrine não renderize um buraco onde deveria haver um glyph.
UPDATE products SET icon = '📦' WHERE icon = '';
