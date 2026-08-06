// PONTO ÚNICO de tokenização de cartão da Appmax — web e balcão compartilham.
//
// PORQUÊ um seam: a Appmax v1 exige que o cartão vire um TOKEN de uso único ANTES
// da cobrança, e a forma correta (PCI SAQ-A) é tokenizar no BROWSER via Appmax JS
// — o PAN nunca toca nosso backend. O backend JÁ aceita `card_token` +
// `installments` (services/payment-service/internal/psp/appmaxv1/gateway.go). O
// que falta é só o passo do browser. Concentramos esse passo AQUI, num único
// lugar, para que quando o contrato/sandbox Appmax chegar (bloqueio do dono)
// tanto o checkout web quanto o balcão passem a cobrar cartão SEM cada um
// reimplementar tokenização. Ver docs/appmax-v1-appstore.md (§ tokenização).
//
// Estado: DORMENTE até `VITE_APPMAX_PUBLIC_KEY` existir. Sem a chave, o app segue
// no fluxo atual (Stripe em dev / mock). E é FAIL-CLOSED: enquanto a chamada real
// do Appmax JS não estiver implementada com o SDK do contrato, RECUSA — nunca
// manda PAN pro servidor nem forja um token.

const PUBLIC_KEY = (import.meta.env.VITE_APPMAX_PUBLIC_KEY as string | undefined) ?? ''

/** true quando a tokenização Appmax no browser está configurada (chave presente). */
export const isAppmaxCardEnabled = PUBLIC_KEY !== ''

export interface AppmaxCardFields {
  /** Dígitos do cartão (com ou sem espaços). */
  number: string
  holderName: string
  /** "MM". */
  expMonth: string
  /** "AA" ou "AAAA". */
  expYear: string
  cvv: string
}

/** Erro de tokenização — o chamador mostra a mensagem ao operador/cliente. */
export class AppmaxTokenizationError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'AppmaxTokenizationError'
  }
}

/**
 * Luhn — barra número de cartão claramente inválido ANTES de qualquer chamada.
 * Não substitui a validação do PSP; é só para não gastar ida ao SDK/rede com um
 * número digitado errado no corredor.
 */
export function isValidCardNumber(raw: string): boolean {
  const digits = raw.replace(/\D/g, '')
  if (digits.length < 13 || digits.length > 19) return false
  let sum = 0
  let double = false
  for (let i = digits.length - 1; i >= 0; i--) {
    let d = digits.charCodeAt(i) - 48
    if (double) {
      d *= 2
      if (d > 9) d -= 9
    }
    sum += d
    double = !double
  }
  return sum % 10 === 0
}

/**
 * PONTO ÚNICO: transforma os dados do cartão num token de uso único da Appmax.
 * É o ÚNICO lugar a mexer quando o SDK Appmax JS entrar (contrato/sandbox).
 *
 * Contrato (o que web e balcão esperam): recebe os campos do cartão e devolve o
 * `card_token`. O chamador passa esse token adiante:
 *   usePayment.createPayment('card', total, { card_token, installments })
 * e o backend cobra via POST /v1/payments/credit-card.
 */
export async function tokenizeCard(fields: AppmaxCardFields): Promise<string> {
  if (!isAppmaxCardEnabled) {
    throw new AppmaxTokenizationError(
      'Tokenização de cartão (Appmax) não configurada — defina VITE_APPMAX_PUBLIC_KEY.'
    )
  }
  if (!isValidCardNumber(fields.number)) {
    throw new AppmaxTokenizationError('Número de cartão inválido.')
  }
  // TODO(appmax-js): carregar o SDK Appmax JS com PUBLIC_KEY (script da Appmax,
  // análogo ao loadStripe em src/lib/stripe.ts) e chamar a tokenização do SDK com
  // `fields`, devolvendo o token de uso único. Enquanto o contrato/sandbox não
  // chega, mantém-se FAIL-CLOSED: recusa em vez de mandar PAN pro backend ou
  // forjar token. Ver docs/appmax-v1-appstore.md.
  throw new AppmaxTokenizationError(
    'Integração de cartão da Appmax pendente (contrato/sandbox). ' +
      'Use Pix, boleto ou maquininha por enquanto.'
  )
}
