/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    container: {
      center: true,
      padding: '1.25rem',
      screens: { '2xl': '1280px' },
    },
    extend: {
      colors: {
        brand: {
          orange: '#F47920',
          'orange-dark': '#D06416',
          'orange-light': '#FFD4A8',
          blue: '#1B3E8A',
          'blue-dark': '#152f6e',
          'blue-light': '#E8EEF8',
          gold: '#F5A623',
        },
      },
      fontFamily: {
        sans: ['Inter', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        display: ['Archivo', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'monospace'],
      },
    },
  },
  plugins: [],
}
