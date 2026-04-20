import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

function PixIcon() {
  return (
    <svg viewBox="0 0 512 512" className="h-6 w-10 fill-current" aria-label="Pix" role="img">
      <path d="M242.4 292.5C247.8 287.1 255.1 284 262.6 284C270.1 284 277.4 287.1 282.8 292.5L354.1 363.7C365.8 375.4 385 375.4 396.7 363.7L428.9 331.5C440.6 319.8 440.6 300.6 428.9 288.9L357.7 217.7C352.3 212.3 349.2 205 349.2 197.5C349.2 190 352.3 182.7 357.7 177.3L428.9 106.1C440.6 94.4 440.6 75.2 428.9 63.5L396.7 31.3C385 19.6 365.8 19.6 354.1 31.3L282.8 102.5C277.4 107.9 270.1 111 262.6 111C255.1 111 247.8 107.9 242.4 102.5L171.2 31.3C159.5 19.6 140.3 19.6 128.6 31.3L96.4 63.5C84.7 75.2 84.7 94.4 96.4 106.1L167.6 177.3C173 182.7 176.1 190 176.1 197.5C176.1 205 173 212.3 167.6 217.7L96.4 288.9C84.7 300.6 84.7 319.8 96.4 331.5L128.6 363.7C140.3 375.4 159.5 375.4 171.2 363.7L242.4 292.5zM354.1 481C365.8 492.7 385 492.7 396.7 481L428.9 448.8C440.6 437.1 440.6 417.9 428.9 406.2L357.7 335C352.3 329.6 349.2 322.3 349.2 314.8C349.2 307.3 352.3 300 357.7 294.6L396.7 255.7C385 244 365.8 244 354.1 255.7L282.8 326.9C277.4 332.3 270.1 335.4 262.6 335.4C255.1 335.4 247.8 332.3 242.4 326.9L171.2 255.7C159.5 244 140.3 244 128.6 255.7L167.6 294.6C173 300 176.1 307.3 176.1 314.8C176.1 322.3 173 329.6 167.6 335L96.4 406.2C84.7 417.9 84.7 437.1 96.4 448.8L128.6 481C140.3 492.7 159.5 492.7 171.2 481L242.4 409.8C247.8 404.4 255.1 401.3 262.6 401.3C270.1 401.3 277.4 404.4 282.8 409.8L354.1 481z" />
    </svg>
  )
}

export function Footer() {
  const { t } = useTranslation()
  const year = new Date().getFullYear()

  return (
    <footer className="bg-brand-blue text-white mt-auto">
      <div className="container py-10">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
          <div>
            <div className="flex items-center gap-2 mb-3">
              <span className="w-8 h-8 rounded-md bg-brand-orange flex items-center justify-center font-display font-black text-lg italic">
                U
              </span>
              <div className="flex flex-col leading-none">
                <span className="font-display font-black text-lg tracking-tight">
                  <span className="text-white">Uti</span>
                  <span className="text-brand-orange">Lar</span>
                </span>
                <span className="text-[9px] font-bold tracking-widest uppercase text-brand-orange-light opacity-80">
                  {t('tagline')}
                </span>
              </div>
            </div>
            <p className="text-sm text-blue-200 leading-relaxed">
              {t('footer.lgpd')}
            </p>
          </div>

          <div>
            <h3 className="text-sm font-semibold text-blue-200 uppercase tracking-wider mb-3">
              Links
            </h3>
            <ul className="flex flex-col gap-2 text-sm">
              {[
                { label: t('footer.about'), href: '/sobre' },
                { label: t('footer.help'), href: '/ajuda' },
                { label: t('footer.contact'), href: '/contato' },
                { label: t('footer.privacyPolicy'), href: '/privacidade' },
                { label: t('footer.terms'), href: '/termos' },
              ].map(({ label, href }) => (
                <li key={href}>
                  <Link to={href} className="text-blue-200 hover:text-white transition-colors">
                    {label}
                  </Link>
                </li>
              ))}
            </ul>
          </div>

          <div>
            <h3 className="text-sm font-semibold text-blue-200 uppercase tracking-wider mb-3">
              {t('footer.paymentMethods')}
            </h3>
            <div className="flex flex-wrap items-center gap-2">
              <span className="flex items-center gap-1 rounded bg-white/10 px-2 py-1 text-xs font-semibold text-white">
                <PixIcon />
                Pix
              </span>
              <span className="rounded bg-white/10 px-2 py-1 text-xs font-semibold text-white">
                Boleto
              </span>
              <span className="rounded bg-white/10 px-2 py-1 text-xs font-semibold text-white">
                Visa
              </span>
              <span className="rounded bg-white/10 px-2 py-1 text-xs font-semibold text-white">
                Mastercard
              </span>
              <span className="rounded bg-white/10 px-2 py-1 text-xs font-semibold text-white">
                Elo
              </span>
            </div>
            <div className="flex items-center gap-3 mt-4">
              <a
                href="https://instagram.com"
                target="_blank"
                rel="noopener noreferrer"
                className="text-blue-200 hover:text-white transition-colors text-sm"
                aria-label="Instagram"
              >
                Instagram
              </a>
              <a
                href="https://wa.me"
                target="_blank"
                rel="noopener noreferrer"
                className="text-blue-200 hover:text-white transition-colors text-sm"
                aria-label="WhatsApp"
              >
                WhatsApp
              </a>
            </div>
          </div>
        </div>

        <div className="mt-8 pt-6 border-t border-white/10 flex flex-col sm:flex-row items-center justify-between gap-2 text-xs text-blue-300">
          <span>
            © {year} UtiLar Ferragem. {t('footer.rights')}.
          </span>
        </div>
      </div>
    </footer>
  )
}
