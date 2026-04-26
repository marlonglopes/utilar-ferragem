import { loadStripe, type Stripe } from '@stripe/stripe-js'

const PUBLISHABLE_KEY = import.meta.env.VITE_STRIPE_PUBLISHABLE_KEY ?? ''

let stripePromise: Promise<Stripe | null> | null = null

export function getStripe(): Promise<Stripe | null> {
  if (!PUBLISHABLE_KEY) {
    return Promise.resolve(null)
  }
  if (!stripePromise) {
    stripePromise = loadStripe(PUBLISHABLE_KEY)
  }
  return stripePromise
}

export const isStripeConfigured = PUBLISHABLE_KEY !== ''
