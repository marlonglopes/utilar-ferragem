-- Reverte 006.
--
-- A conta só pode sair se NENHUMA partida a referencia — ledger_entries tem FK
-- para ledger_accounts(code) e o livro é imutável por trigger. Se já houve
-- liquidação externa, este DELETE falha, e falhar é o comportamento certo:
-- apagar a contrapartida de lançamentos existentes deixaria o livro sem
-- descrever a própria conta. Nesse caso, reverta a feature na aplicação e
-- deixe a conta em pé — conta sem movimento novo não faz mal a ninguém.
DELETE FROM ledger_accounts
 WHERE code = '1.1.3'
   AND NOT EXISTS (SELECT 1 FROM ledger_entries WHERE account_code = '1.1.3');
