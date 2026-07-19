// ⚠️ DADOS CADASTRAIS DA EMPRESA — PENDENTE DE PREENCHIMENTO
//
// Todos os campos marcados com [PREENCHER: ...] são placeholders e DEVEM ser
// substituídos pelos dados reais antes do site ir ao ar. O Decreto 7.962/2013
// (art. 2º, I e II) obriga o e-commerce a exibir de forma ostensiva razão
// social, CNPJ e endereço físico e eletrônico do fornecedor.
//
// Centralizado aqui para que a revisão jurídica precise editar um arquivo só —
// as páginas de Termos, Privacidade, Contato e o rodapé leem daqui.

export const COMPANY = {
  /** Nome fantasia usado na comunicação com o cliente. */
  tradeName: 'UtiLar Ferragem',
  /** Razão social registrada na Receita Federal. */
  legalName: '[PREENCHER: RAZÃO SOCIAL]',
  cnpj: '[PREENCHER: CNPJ]',
  /** Inscrição estadual — obrigatória para quem comercializa mercadoria. */
  stateRegistration: '[PREENCHER: INSCRIÇÃO ESTADUAL]',
  address: {
    street: '[PREENCHER: LOGRADOURO, NÚMERO]',
    complement: '[PREENCHER: COMPLEMENTO]',
    neighborhood: '[PREENCHER: BAIRRO]',
    city: '[PREENCHER: CIDADE]',
    state: '[PREENCHER: UF]',
    zip: '[PREENCHER: CEP]',
  },
  contact: {
    email: '[PREENCHER: E-MAIL DE ATENDIMENTO]',
    /** Encarregado pelo tratamento de dados (DPO) — LGPD art. 41. */
    dpoName: '[PREENCHER: NOME DO ENCARREGADO (DPO)]',
    dpoEmail: '[PREENCHER: E-MAIL DO ENCARREGADO (DPO)]',
    phone: '[PREENCHER: TELEFONE]',
    whatsapp: '[PREENCHER: WHATSAPP]',
    /** Horário de atendimento exibido na página de contato. */
    hours: 'Segunda a sexta, das 8h às 18h (exceto feriados)',
  },
  /** Comarca eleita para dirimir conflitos — cláusula de foro dos Termos. */
  jurisdiction: '[PREENCHER: COMARCA/UF DO FORO]',
  /** Data da última revisão dos documentos legais. */
  legalDocsUpdatedAt: '[PREENCHER: DATA DA REVISÃO JURÍDICA]',
} as const

/** Endereço completo em uma linha, para exibição no rodapé e nos documentos legais. */
export function formattedAddress(): string {
  const { street, complement, neighborhood, city, state, zip } = COMPANY.address
  return `${street}, ${complement} — ${neighborhood}, ${city}/${state}, CEP ${zip}`
}
